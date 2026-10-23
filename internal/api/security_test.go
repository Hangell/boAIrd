package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestSecurityHeadersApplied(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, header := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		if response.Header().Get(header) == "" {
			t.Fatalf("security header %q not set", header)
		}
	}
	if response.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS emitted for a non-TLS request")
	}
}

func TestSecurityHeadersHSTSOnlyForTLS(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.TLS = &tls.ConnectionState{}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("HSTS not emitted for a TLS request")
	}
}

func TestRateLimitReturns429(t *testing.T) {
	handler := rateLimitHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), SecurityPolicy{RateLimitRequests: 3, RateLimitWindow: time.Minute})
	for i := 0; i < 3; i++ {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("request %d status = %d, want 204", i+1, response.Code)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil))
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limited status = %d, want 429", response.Code)
	}
}

func TestClientIPHonorsOnlyTrustedProxies(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "10.0.0.5:443"
	request.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.9")
	if got := clientIP(request, trusted); got != "198.51.100.7" {
		t.Fatalf("clientIP = %q, want 198.51.100.7", got)
	}

	untrusted := httptest.NewRequest(http.MethodGet, "/", nil)
	untrusted.RemoteAddr = "198.51.100.1:443"
	untrusted.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := clientIP(untrusted, trusted); got != "198.51.100.1" {
		t.Fatalf("clientIP = %q, want direct peer 198.51.100.1", got)
	}

	plain := httptest.NewRequest(http.MethodGet, "/", nil)
	plain.RemoteAddr = "192.0.2.10:443"
	if got := clientIP(plain, nil); got != "192.0.2.10" {
		t.Fatalf("clientIP = %q, want 192.0.2.10", got)
	}
}
