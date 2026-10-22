package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"boAIrd/internal/credential"
	"boAIrd/internal/provider"
)

type testCredentials struct{}

func (testCredentials) Resolve(context.Context, string, credential.Scope) (string, error) {
	return "anthropic-secret", nil
}
func TestChatSeparatesSystemAndNormalizesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "anthropic-secret" || r.Header.Get("Anthropic-Version") == "" {
			t.Errorf("missing Anthropic headers")
		}
		var body struct {
			System   string `json:"system"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
			MaxTokens   int      `json:"max_tokens"`
			Temperature *float64 `json:"temperature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.System != "instructions" || len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.MaxTokens != 24 || body.Temperature != nil {
			t.Errorf("unexpected request: %+v", body)
		}
		_, _ = w.Write([]byte(`{"id":"m1","model":"claude-test","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":5}}`))
	}))
	defer server.Close()
	adapter, err := New(provider.Config{BaseURL: server.URL, CredentialID: "ref", CredentialResolver: testCredentials{}})
	if err != nil {
		t.Fatal(err)
	}
	defaultTemperature := 1.0
	response, err := adapter.Chat(context.Background(), provider.ChatRequest{Model: "claude-test", MaxTokens: 24, Temperature: &defaultTemperature, Messages: []provider.ChatMessage{{Role: provider.RoleSystem, Content: "instructions"}, {Role: provider.RoleUser, Content: "question"}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "hello" || response.ID != "m1" || response.Usage.OutputTokens != 5 {
		t.Fatalf("unexpected response: %+v", response)
	}
	temperature := 0.4
	_, err = adapter.Chat(context.Background(), provider.ChatRequest{Model: "claude-test", MaxTokens: 24, Temperature: &temperature, Messages: []provider.ChatMessage{{Role: provider.RoleUser, Content: "question"}}})
	if !errors.Is(err, provider.ErrInvalidRequest) {
		t.Fatalf("expected unsupported Anthropic temperature to fail, got %v", err)
	}
}
