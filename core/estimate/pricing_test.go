package estimate

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRateLookupCurrencyCacheAndCredentialHandling(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if !strings.Contains(string(body), "prices{EUR}") || strings.Contains(string(body), "private-key") {
			t.Error("query must request EUR without embedding credentials")
		}
		if r.Header.Get("X-Api-Key") != "private-key" {
			t.Error("missing API authentication")
		}
		_, _ = w.Write([]byte(`{"data":{"products":[{"prices":[{"EUR":"0.0223125"}]}]}}`))
	}))
	defer server.Close()
	lookup := newRateLookup(server.Client(), server.URL, "private-key", "EUR")
	for i := 0; i < 2; i++ {
		n, err := lookup("global", "Compute Engine", "minimum")
		if err != nil || n != .0223125 {
			t.Fatalf("got %v, %v", n, err)
		}
	}
	if calls != 1 {
		t.Fatalf("queried same SKU %d times", calls)
	}
}

func TestRateLookupFailsOnMissingAmbiguousOrInvalidPrices(t *testing.T) {
	for _, body := range []string{`{"errors":[{"message":"private-key"}]}`, `{"data":{"products":[]}}`, `{"data":{"products":[{"prices":[{"USD":"NaN"}]}]}}`, `{"data":{"products":[{"prices":[{"USD":"1"},{"USD":"2"}]}]}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		_, err := newRateLookup(server.Client(), server.URL, "private-key", "USD")("global", "Compute Engine", "minimum")
		server.Close()
		if err == nil {
			t.Fatalf("accepted %s", body)
		}
		if strings.Contains(err.Error(), "private-key") {
			t.Fatal("credential leaked into diagnostic")
		}
	}
}

func TestConservativeTierLookupExcludesFreeAllowances(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"products":[{"prices":[{"USD":"0.12"},{"USD":"0"},{"USD":"0.11"},{"USD":"0.08"}]}]}}`))
	}))
	defer server.Close()
	price, err := newRateLookup(server.Client(), server.URL, "private-key", "USD")("us-central1", "Compute Engine", "/^Network Internet Data Transfer Out from Americas to Americas$/i")
	if err != nil || price != .12 {
		t.Fatalf("expected conservative first paid tier, got %v, %v", price, err)
	}
}
