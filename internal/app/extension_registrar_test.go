package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"boAIrd/internal/capability"
	"boAIrd/internal/extension"
	"boAIrd/internal/provider"
	governedtool "boAIrd/internal/tool"
)

type extensionAuditorStub struct{}

func (extensionAuditorStub) RecordToolExecution(_ context.Context, _ governedtool.Event) error {
	return nil
}

type extensionRoundTripper func(*http.Request) (*http.Response, error)

func (f extensionRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestExtensionHTTPToolValidatesSchemasEnforcesCapabilityAndRedacts(t *testing.T) {
	policy, err := provider.NewEndpointPolicy([]string{"api.example.com"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := governedtool.NewRegistry()
	registrar := newExtensionToolRegistrar(registry, policy)
	registrar.client.Transport = extensionRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "api.example.com" || request.URL.Path != "/search" {
			t.Fatalf("request target = %s, want approved host/path", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"token":"sk-1234567890abcdef","ok":true}`)), Request: request}, nil
	})
	declaration := extension.ToolDeclaration{
		Name: "example.search", Description: "Search", Kind: extension.KindHTTP, Endpoint: "https://api.example.com/search",
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"q":{"type":"string","minLength":1}},"required":["q"],"additionalProperties":false}`),
		OutputSchema:         json.RawMessage(`{"type":"object","properties":{"token":{"type":"string"},"ok":{"type":"boolean"}},"required":["token","ok"],"additionalProperties":false}`),
		RequiredCapabilities: []capability.Capability{"example.search"},
	}
	if err := registrar.RegisterTools(context.Background(), []extension.ToolDeclaration{declaration}); err != nil {
		t.Fatal(err)
	}
	enforcer, err := governedtool.NewEnforcer(registry, extensionAuditorStub{}, governedtool.ScopeAuthorizerFunc(func(_ context.Context, principal governedtool.Principal, request governedtool.Request) error {
		if principal.ID == "agent-1" && request.ProjectID == "project-1" && request.ResourceType == "extension" && request.ResourceID == request.ToolName {
			return nil
		}
		return governedtool.ErrDenied
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := governedtool.Request{
		ToolName: "example.search", ProjectID: "project-1", ResourceType: "extension", ResourceID: "example.search",
		Principal: governedtool.Principal{ID: "agent-1", Type: "agent", Capabilities: []capability.Capability{"example.search"}, ProjectIDs: []string{"project-1"}},
		Input:     json.RawMessage(`{"q":"hello"}`),
	}
	encoded, err := enforcer.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(string(encoded), "sk-1234567890abcdef") {
		t.Fatalf("extension output leaks a token: %s", encoded)
	}
	request.Input = json.RawMessage(`{"q":""}`)
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, governedtool.ErrInvalidInput) {
		t.Fatalf("Execute() with schema-invalid input error = %v, want invalid input", err)
	}
	request.Input = json.RawMessage(`{"q":"hello"}`)
	request.Principal.Capabilities = nil
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, governedtool.ErrDenied) {
		t.Fatalf("Execute() without capability error = %v, want denied", err)
	}
}

func TestExtensionHTTPToolRejectsRedirectAndUnapprovedEndpoint(t *testing.T) {
	policy, err := provider.NewEndpointPolicy([]string{"api.example.com"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := governedtool.NewRegistry()
	registrar := newExtensionToolRegistrar(registry, policy)
	calls := 0
	registrar.client.Transport = extensionRoundTripper(func(request *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://evil.example/steal"}}, Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
	})
	declaration := extension.ToolDeclaration{
		Name: "example.search", Description: "Search", Kind: extension.KindHTTP, Endpoint: "https://api.example.com/search",
		InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`),
		RequiredCapabilities: []capability.Capability{"example.search"},
	}
	if err := registrar.RegisterTools(context.Background(), []extension.ToolDeclaration{declaration}); err != nil {
		t.Fatal(err)
	}
	enforcer, err := governedtool.NewEnforcer(registry, extensionAuditorStub{}, governedtool.ScopeAuthorizerFunc(func(context.Context, governedtool.Principal, governedtool.Request) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	_, err = enforcer.Execute(context.Background(), governedtool.Request{
		ToolName: "example.search", ProjectID: "project-1", ResourceType: "extension", ResourceID: "example.search",
		Principal: governedtool.Principal{ID: "agent-1", Type: "agent", Capabilities: []capability.Capability{"example.search"}, ProjectIDs: []string{"project-1"}},
		Input:     json.RawMessage(`{}`),
	})
	if !errors.Is(err, governedtool.ErrExecution) {
		t.Fatalf("redirect execution error = %v, want execution failure", err)
	}
	if calls != 1 {
		t.Fatalf("HTTP transport calls = %d, want exactly one", calls)
	}
	declaration.Endpoint = "https://evil.example/search"
	if err := registrar.RegisterTools(context.Background(), []extension.ToolDeclaration{declaration}); err == nil {
		t.Fatal("unapproved endpoint was registered")
	}
}

func TestExtensionDeclarativeToolIsNotExecutable(t *testing.T) {
	policy, err := provider.NewEndpointPolicy(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := governedtool.NewRegistry()
	registrar := newExtensionToolRegistrar(registry, policy)
	declaration := extension.ToolDeclaration{
		Name: "contract.only", Description: "Contract only", Kind: extension.KindDeclarative,
		InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`),
		RequiredCapabilities: []capability.Capability{"contract.read"},
	}
	if err := registrar.RegisterTools(context.Background(), []extension.ToolDeclaration{declaration}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("contract.only"); !errors.Is(err, governedtool.ErrNotFound) {
		t.Fatalf("declarative tool was registered as executable; Resolve() error = %v", err)
	}
}
