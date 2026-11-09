package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domeneshop "github.com/vidarno/external-dns-domeneshop-webhook/internal/client"
	"sigs.k8s.io/external-dns/endpoint"
)

func testDomains() []domeneshop.Domain {
	mk := func(id int, name string, dns bool) domeneshop.Domain {
		d := domeneshop.Domain{ID: id, Name: name}
		d.Services.DNS = dns
		return d
	}
	return []domeneshop.Domain{
		mk(1, "example.com", true),
		mk(2, "sub.example.com", true),
		mk(3, "example.no", true),
		mk(4, "nodns.org", false),
	}
}

func TestFindDomainZone(t *testing.T) {
	tests := []struct {
		name   string
		want   string
		wantOK bool
	}{
		{"example.com", "example.com", true},
		{"example.com.", "example.com", true},
		{"www.example.com", "example.com", true},
		{"a.b.c.example.com", "example.com", true},
		{"www.sub.example.com", "sub.example.com", true},
		{"sub.example.com", "sub.example.com", true},
		{"WWW.Example.NO", "example.no", true},
		{"badexample.com", "", false},
		{"unknown.net", "", false},
		{"com", "", false},
	}
	for _, tt := range tests {
		got, ok := findDomainZone(testDomains(), tt.name)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("findDomainZone(%q) = (%q, %v), want (%q, %v)", tt.name, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestHostConversion(t *testing.T) {
	if got := fqdnToHost("example.com", "example.com"); got != "@" {
		t.Errorf("apex host = %q, want @", got)
	}
	if got := fqdnToHost("www.example.com.", "example.com"); got != "www" {
		t.Errorf("host = %q, want www", got)
	}
	if got := fqdnToHost("a.b.example.com", "example.com"); got != "a.b" {
		t.Errorf("host = %q, want a.b", got)
	}
	if got := hostToFQDN("@", "example.com"); got != "example.com" {
		t.Errorf("apex fqdn = %q, want example.com", got)
	}
	if got := hostToFQDN("www", "example.com"); got != "www.example.com" {
		t.Errorf("fqdn = %q, want www.example.com", got)
	}
}

func TestEndpointToDnsRecord(t *testing.T) {
	ep := endpoint.NewEndpointWithTTL("example.com", "A", 0, "1.2.3.4")
	r := endpointToDnsRecord("example.com", ep, "1.2.3.4")
	if r.Host != "@" || r.TTL != 3600 || r.Data != "1.2.3.4" || r.Type != "A" {
		t.Errorf("unexpected record %+v", r)
	}
	ep = endpoint.NewEndpointWithTTL("app.example.com", "CNAME", 300, "target.example.no")
	r = endpointToDnsRecord("example.com", ep, "target.example.no")
	if r.Host != "app" || r.TTL != 300 {
		t.Errorf("unexpected record %+v", r)
	}
}

func newTestProvider(t *testing.T, records map[int][]domeneshop.DNSRecord, failRecords bool) *Provider {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/domains", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(testDomains())
	})
	mux.HandleFunc("/domains/", func(w http.ResponseWriter, r *http.Request) {
		if failRecords {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		var id int
		switch r.URL.Path {
		case "/domains/1/dns":
			id = 1
		case "/domains/2/dns":
			id = 2
		case "/domains/3/dns":
			id = 3
		default:
			http.NotFound(w, r)
			return
		}
		recs := records[id]
		if recs == nil {
			recs = []domeneshop.DNSRecord{}
		}
		_ = json.NewEncoder(w).Encode(recs)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	p := NewProvider("token", "secret")
	p.domeneshopClient.BaseURL = srv.URL
	return p
}

func TestRecords(t *testing.T) {
	p := newTestProvider(t, map[int][]domeneshop.DNSRecord{
		1: {
			{ID: 1, Host: "@", Type: "A", Data: "1.1.1.1", TTL: 3600},
			{ID: 2, Host: "www", Type: "A", Data: "1.1.1.1", TTL: 300},
			{ID: 3, Host: "www", Type: "A", Data: "2.2.2.2", TTL: 300},
		},
		// domain 2 has no records, which must not be an error
		3: {
			{ID: 4, Host: "txt", Type: "TXT", Data: "hello", TTL: 3600},
		},
	}, false)

	eps, err := p.Records()
	if err != nil {
		t.Fatalf("Records() error: %v", err)
	}
	if len(eps) != 3 {
		t.Fatalf("got %d endpoints, want 3: %v", len(eps), eps)
	}
	byName := map[string]*endpoint.Endpoint{}
	for _, ep := range eps {
		byName[ep.DNSName+"/"+ep.RecordType] = ep
	}
	if ep := byName["example.com/A"]; ep == nil || !ep.Targets.Same(endpoint.Targets{"1.1.1.1"}) {
		t.Errorf("apex endpoint wrong: %v", ep)
	}
	if ep := byName["www.example.com/A"]; ep == nil || !ep.Targets.Same(endpoint.Targets{"1.1.1.1", "2.2.2.2"}) || ep.RecordTTL != 300 {
		t.Errorf("www endpoint wrong: %v", ep)
	}
	if ep := byName["txt.example.no/TXT"]; ep == nil {
		t.Errorf("txt endpoint missing")
	}
}

func TestRecordsAPIError(t *testing.T) {
	p := newTestProvider(t, nil, true)
	if _, err := p.Records(); err == nil {
		t.Fatal("expected error from Records() when API fails")
	}
}

func TestGetDomainZoneTerminates(t *testing.T) {
	// Previously this looped forever for names that did not match on the first attempt.
	p := newTestProvider(t, nil, false)
	if zone, ok := getDomainZone(p.domeneshopClient, "a.b.c.example.no"); !ok || zone != "example.no" {
		t.Errorf("got (%q, %v), want (example.no, true)", zone, ok)
	}
	if _, ok := getDomainZone(p.domeneshopClient, "nodns.org"); ok {
		t.Errorf("domain without DNS service must not match")
	}
}
