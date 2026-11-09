package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	domeneshop "github.com/vidarno/external-dns-domeneshop-webhook/internal/client"
	domeneshopProvider "github.com/vidarno/external-dns-domeneshop-webhook/internal/provider"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/pkg/apis/externaldns"
	"sigs.k8s.io/external-dns/plan"
	externaldnswebhook "sigs.k8s.io/external-dns/provider/webhook"
)

// fakeDomeneshop is a minimal in-memory implementation of the Domeneshop API
type fakeDomeneshop struct {
	mu      sync.Mutex
	nextID  int
	domains []domeneshop.Domain
	records map[int][]domeneshop.DNSRecord
}

func newFakeDomeneshop() *fakeDomeneshop {
	d := domeneshop.Domain{ID: 1, Name: "example.com"}
	d.Services.DNS = true
	return &fakeDomeneshop{
		nextID:  100,
		domains: []domeneshop.Domain{d},
		records: map[int][]domeneshop.DNSRecord{
			1: {{ID: 1, Host: "@", Type: "A", Data: "1.1.1.1", TTL: 3600}},
		},
	}
}

func (f *fakeDomeneshop) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 1 && parts[0] == "domains" {
		_ = json.NewEncoder(w).Encode(f.domains)
		return
	}
	if len(parts) < 3 || parts[0] != "domains" || parts[2] != "dns" {
		http.NotFound(w, r)
		return
	}
	domainID, _ := strconv.Atoi(parts[1])

	switch {
	case len(parts) == 3 && r.Method == http.MethodGet:
		recs := f.records[domainID]
		if recs == nil {
			recs = []domeneshop.DNSRecord{}
		}
		_ = json.NewEncoder(w).Encode(recs)
	case len(parts) == 3 && r.Method == http.MethodPost:
		var rec domeneshop.DNSRecord
		if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.nextID++
		rec.ID = f.nextID
		f.records[domainID] = append(f.records[domainID], rec)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]int{"id": rec.ID})
	case len(parts) == 4:
		recordID, _ := strconv.Atoi(parts[3])
		recs := f.records[domainID]
		for i, rec := range recs {
			if rec.ID != recordID {
				continue
			}
			switch r.Method {
			case http.MethodPut:
				var upd domeneshop.DNSRecord
				if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				upd.ID = recordID
				recs[i] = upd
			case http.MethodDelete:
				f.records[domainID] = append(recs[:i], recs[i+1:]...)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	default:
		http.Error(w, "unsupported", http.StatusMethodNotAllowed)
	}
}

// TestExternalDNSWebhookClient drives the webhook through the real external-dns webhook client
func TestExternalDNSWebhookClient(t *testing.T) {
	api := httptest.NewServer(newFakeDomeneshop())
	defer api.Close()

	client := domeneshop.NewClient("token", "secret")
	client.BaseURL = api.URL
	wh := NewWithProvider(domeneshopProvider.NewProviderWithClient(client))

	mux := http.NewServeMux()
	mux.HandleFunc("/", wh.DomainFilter)
	mux.HandleFunc("/records", wh.Records)
	mux.HandleFunc("/adjustendpoints", wh.AdjustEndpoints)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	cfg := &externaldns.Config{
		WebhookProviderURL:          srv.URL,
		WebhookProviderReadTimeout:  5 * time.Second,
		WebhookProviderWriteTimeout: 5 * time.Second,
		WebhookProviderMaxBodySize:  1 << 20,
	}
	p, err := externaldnswebhook.New(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("negotiation failed: %v", err)
	}

	records := func() map[string]*endpoint.Endpoint {
		t.Helper()
		eps, err := p.Records(ctx)
		if err != nil {
			t.Fatalf("Records() failed: %v", err)
		}
		m := map[string]*endpoint.Endpoint{}
		for _, ep := range eps {
			m[ep.DNSName+"/"+ep.RecordType] = ep
		}
		return m
	}

	if ep := records()["example.com/A"]; ep == nil || !ep.Targets.Same(endpoint.Targets{"1.1.1.1"}) {
		t.Fatalf("apex record not returned correctly: %v", ep)
	}

	adjusted, err := p.AdjustEndpoints([]*endpoint.Endpoint{endpoint.NewEndpoint("www.example.com", "A", "2.2.2.2")})
	if err != nil || len(adjusted) != 1 {
		t.Fatalf("AdjustEndpoints() = %v, %v", adjusted, err)
	}

	www := endpoint.NewEndpointWithTTL("www.example.com", "A", 300, "2.2.2.2", "3.3.3.3")
	if err := p.ApplyChanges(ctx, &plan.Changes{Create: []*endpoint.Endpoint{www}}); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if ep := records()["www.example.com/A"]; ep == nil || !ep.Targets.Same(endpoint.Targets{"2.2.2.2", "3.3.3.3"}) || ep.RecordTTL != 300 {
		t.Fatalf("created record wrong: %v", ep)
	}

	wwwNew := endpoint.NewEndpointWithTTL("www.example.com", "A", 600, "3.3.3.3", "4.4.4.4")
	if err := p.ApplyChanges(ctx, &plan.Changes{UpdateOld: []*endpoint.Endpoint{www}, UpdateNew: []*endpoint.Endpoint{wwwNew}}); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if ep := records()["www.example.com/A"]; ep == nil || !ep.Targets.Same(endpoint.Targets{"3.3.3.3", "4.4.4.4"}) || ep.RecordTTL != 600 {
		t.Fatalf("updated record wrong: %v", ep)
	}

	if err := p.ApplyChanges(ctx, &plan.Changes{Delete: []*endpoint.Endpoint{wwwNew}}); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if ep := records()["www.example.com/A"]; ep != nil {
		t.Fatalf("record not deleted: %v", ep)
	}
}
