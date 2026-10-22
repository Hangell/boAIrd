package deepseek

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"boAIrd/internal/credential"
	"boAIrd/internal/provider"
)

type testCredentials struct{}

func (testCredentials) Resolve(context.Context, string, credential.Scope) (string, error) {
	return "deepseek-secret", nil
}

func TestModelsMapsProviderMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer deepseek-secret" {
			t.Error("missing bearer credential")
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"deepseek-flash","object":"model","owned_by":"deepseek","name":"DeepSeek Flash","context_window":1048576,"max_output_tokens":393216,"input_modalities":["text","image"],"output_modalities":["text"],"effort":{"supported_levels":["low","high","max"],"default_level":"high"}}]}`))
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
	if len(models) != 1 {
		t.Fatalf("got %d models, want one", len(models))
	}
	model := models[0]
	if model.ID != "deepseek-flash" || model.Name != "DeepSeek Flash" || model.OwnedBy != "deepseek" || model.ContextWindow != 1048576 || model.MaxOutputTokens != 393216 || !reflect.DeepEqual(model.InputModalities, []string{"text", "image"}) || !reflect.DeepEqual(model.OutputModalities, []string{"text"}) || !reflect.DeepEqual(model.SupportedEffortLevels, []string{"low", "high", "max"}) || model.DefaultEffortLevel != "high" {
		t.Fatalf("model metadata was not preserved: %+v", model)
	}
}
