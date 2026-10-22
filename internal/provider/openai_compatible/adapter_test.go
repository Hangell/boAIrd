package openai_compatible

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boAIrd/internal/provider"
)

type credentials struct{}

func (credentials) Resolve(context.Context, string) (string, error) { return "secret-value", nil }

func TestChatMapsRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-value" {
			t.Errorf("authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["max_tokens"] != float64(32) {
			t.Errorf("max_tokens = %v", body["max_tokens"])
		}
		if _, exists := body["max_completion_tokens"]; exists {
			t.Error("unexpected max_completion_tokens")
		}
		_, _ = w.Write([]byte(`{"id":"chat-1","model":"fake","choices":[{"message":{"content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`))
	}))
	defer server.Close()
	adapter, err := New(ConfigForTest(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Chat(context.Background(), provider.ChatRequest{Model: "fake", MaxTokens: 32, Messages: []provider.ChatMessage{{Role: provider.RoleUser, Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "answer" || response.Usage.InputTokens != 4 || response.Usage.OutputTokens != 2 || response.FinishReason != "stop" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestHTTPErrorDoesNotExposeProviderBodyOrCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("secret-value provider diagnostic"))
	}))
	defer server.Close()
	adapter, err := New(ConfigForTest(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Models(context.Background())
	var httpErr *provider.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected sanitized HTTP error, got %v", err)
	}
	if strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "provider diagnostic") {
		t.Fatalf("error leaked response content: %v", err)
	}
}

func TestChatHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	adapter, err := New(ConfigForTest(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = adapter.Chat(ctx, provider.ChatRequest{Model: "fake", MaxTokens: 8, Messages: []provider.ChatMessage{{Role: provider.RoleUser, Content: "hello"}}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func ConfigForTest(baseURL string) provider.Config {
	return provider.Config{BaseURL: baseURL, CredentialID: "credential", CredentialResolver: credentials{}}
}
