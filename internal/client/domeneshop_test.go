package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetDomainsReturnsEmptySlice(t *testing.T) {
	originalAPIURL := apiURL
	defer func() { apiURL = originalAPIURL }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/domains" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`[]`)); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	defer srv.Close()

	apiURL = srv.URL

	client := NewClient("token", "secret")
	domains, err := client.GetDomains()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(domains) != 0 {
		t.Fatalf("expected no domains, got %d", len(domains))
	}
}

func TestGetRecordsReturnsEmptySlice(t *testing.T) {
	originalAPIURL := apiURL
	defer func() { apiURL = originalAPIURL }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/domains/42/dns" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`[]`)); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	defer srv.Close()

	apiURL = srv.URL

	client := NewClient("token", "secret")
	records, err := client.GetRecords(42)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no records, got %d", len(records))
	}
}
