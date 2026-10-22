package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type testCredentialResolver struct {
	value string
	err   error
}

func (r testCredentialResolver) Resolve(context.Context, string) (string, error) {
	return r.value, r.err
}

func TestNewJSONClientRejectsUnsafeBaseURLs(t *testing.T) {
	invalidURLs := []string{
		"",
		"file:///tmp/provider",
		"http://provider.example/v1",
		"https://user:password@provider.example/v1",
		"https://provider.example/v1?token=secret",
		"https://provider.example/v1#fragment",
		"https://provider.example/v1/../admin",
		"https://provider.example/v1/%2e%2e/admin",
	}
	for _, baseURL := range invalidURLs {
		t.Run(baseURL, func(t *testing.T) {
			_, err := NewJSONClient("test", Config{BaseURL: baseURL, CredentialID: "credential", CredentialResolver: testCredentialResolver{value: "secret"}}, "", "Authorization", "Bearer ")
			if !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("NewJSONClient(%q) error = %v, want ErrInvalidConfiguration", baseURL, err)
			}
		})
	}
}

func TestNewJSONClientAllowsHTTPSAndLoopbackHTTP(t *testing.T) {
	for _, baseURL := range []string{"https://provider.example/v1", "http://127.0.0.1:8080/v1", "http://[::1]:8080/v1", "http://localhost:8080/v1"} {
		t.Run(baseURL, func(t *testing.T) {
			_, err := NewJSONClient("test", Config{BaseURL: baseURL, CredentialID: "credential", CredentialResolver: testCredentialResolver{value: "secret"}}, "", "Authorization", "Bearer ")
			if err != nil {
				t.Fatalf("NewJSONClient(%q): %v", baseURL, err)
			}
		})
	}
}

func TestRedirectDoesNotForwardCredential(t *testing.T) {
	var destinationRequests atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationRequests.Add(1)
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("redirect forwarded authorization header %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer destination.Close()

	const secret = "secret-value"
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Errorf("origin authorization = %q", got)
		}
		if strings.Contains(r.URL.String(), secret) {
			t.Errorf("credential appeared in request URL %q", r.URL.String())
		}
		http.Redirect(w, r, destination.URL+"/capture?key="+secret, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	client, err := NewJSONClient("test", Config{BaseURL: origin.URL, CredentialID: "credential", CredentialResolver: testCredentialResolver{value: secret}}, "", "Authorization", "Bearer ")
	if err != nil {
		t.Fatal(err)
	}
	err = client.Do(context.Background(), http.MethodGet, "models", nil, nil, nil, nil)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("Do error = %v, want HTTP 307", err)
	}
	if destinationRequests.Load() != 0 {
		t.Fatalf("redirect destination received %d requests", destinationRequests.Load())
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("redirect error leaked credential: %v", err)
	}
}

func TestCredentialResolverErrorDoesNotLeakSecret(t *testing.T) {
	const secret = "credential-secret"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	client, err := NewJSONClient("test", Config{BaseURL: server.URL, CredentialID: "credential", CredentialResolver: testCredentialResolver{err: errors.New("vault failed for " + secret)}}, "", "Authorization", "Bearer ")
	if err != nil {
		t.Fatal(err)
	}
	err = client.Do(context.Background(), http.MethodGet, "models", nil, nil, nil, nil)
	if !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("Do error = %v, want ErrCredentialUnavailable", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("resolver error leaked credential: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("request was sent when credential resolution failed")
	}
}
