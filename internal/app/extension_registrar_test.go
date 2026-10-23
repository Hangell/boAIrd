package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boAIrd/internal/capability"
	"boAIrd/internal/extension"
	governedtool "boAIrd/internal/tool"
)

type extensionAuditorStub struct{}

func (extensionAuditorStub) RecordToolExecution(_ context.Context, _ governedtool.Event) error {
	return nil
}

func TestExtensionHTTPToolExecutesWithCapabilityEnforcementAndRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"sk-1234567890abcdef","ok":true}`))
	}))
	t.Cleanup(server.Close)

	registry := governedtool.NewRegistry()
	registrar := newExtensionToolRegistrar(registry)
	declaration := extension.ToolDeclaration{
		Name: "example.search", Description: "Search", Kind: extension.KindHTTP, Endpoint: server.URL,
		InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`),
		RequiredCapabilities: []capability.Capability{"example.search"},
	}
	if err := registrar.RegisterTool(context.Background(), declaration); err != nil {
		t.Fatal(err)
	}
	auditor := extensionAuditorStub{}
	enforcer, err := governedtool.NewEnforcer(registry, auditor, governedtool.ScopeAuthorizerFunc(func(_ context.Context, principal governedtool.Principal, request governedtool.Request) error {
		if principal.ID == "agent-1" {
			return nil
		}
		return governedtool.ErrDenied
	}))
	if err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]any{"q": "hello"})
	request := governedtool.Request{
		ToolName: "example.search", ProjectID: "project-1", ResourceType: "extension", ResourceID: "example.search",
		Principal: governedtool.Principal{ID: "agent-1", Type: "agent", Capabilities: []capability.Capability{"example.search"}, ProjectIDs: []string{"project-1"}},
		Input:     input,
	}
	encoded, err := enforcer.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(string(encoded), "sk-1234567890abcdef") {
		t.Fatalf("extension output leaks a token: %s", encoded)
	}
	// Denied without the required capability.
	request.Principal.Capabilities = nil
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, governedtool.ErrDenied) {
		t.Fatalf("Execute() without capability error = %v, want denied", err)
	}
}

func TestExtensionDeclarativeToolIsNotExecutable(t *testing.T) {
	registry := governedtool.NewRegistry()
	registrar := newExtensionToolRegistrar(registry)
	declaration := extension.ToolDeclaration{
		Name: "contract.only", Description: "Contract only", Kind: extension.KindDeclarative,
		InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`),
		RequiredCapabilities: []capability.Capability{"contract.read"},
	}
	if err := registrar.RegisterTool(context.Background(), declaration); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("contract.only"); !errors.Is(err, governedtool.ErrNotFound) {
		t.Fatalf("declarative tool was registered as executable; Resolve() error = %v", err)
	}
}
