package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boAIrd/internal/provider"
)

type testCredentials struct{}

func (testCredentials) Resolve(context.Context, string) (string, error) { return "gemini-secret", nil }
func TestModelsFiltersAndPaginates(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("x-goog-api-key") != "gemini-secret" {
			t.Error("missing API key header")
		}
		if requests == 1 {
			_, _ = w.Write([]byte(`{"models":[{"name":"models/embedding","supportedGenerationMethods":["embedContent"]}],"nextPageToken":"next"}`))
			return
		}
		if r.URL.Query().Get("pageToken") != "next" {
			t.Errorf("pageToken = %q", r.URL.Query().Get("pageToken"))
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-test","displayName":"Gemini Test","inputTokenLimit":1000,"outputTokenLimit":100,"supportedGenerationMethods":["generateContent"]}]}`))
	}))
	defer server.Close()
	adapter, err := New(provider.Config{BaseURL: server.URL, CredentialID: "ref", CredentialResolver: testCredentials{}})
	if err != nil {
		t.Fatal(err)
	}
	models, err := adapter.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(models) != 1 || models[0].ID != "gemini-test" || models[0].ContextWindow != 1000 {
		t.Fatalf("unexpected models: %+v; requests=%d", models, requests)
	}
}
func TestChatUsesGenerateContentContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-test:generateContent" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["systemInstruction"]; !ok {
			t.Error("systemInstruction missing")
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":2}}`))
	}))
	defer server.Close()
	adapter, err := New(provider.Config{BaseURL: server.URL, CredentialID: "ref", CredentialResolver: testCredentials{}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Chat(context.Background(), provider.ChatRequest{Model: "gemini-test", MaxTokens: 20, Messages: []provider.ChatMessage{{Role: provider.RoleSystem, Content: "rules"}, {Role: provider.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "ok" || response.Usage.InputTokens != 7 {
		t.Fatalf("unexpected response: %+v", response)
	}
}
