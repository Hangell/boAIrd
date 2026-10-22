package provider_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"boAIrd/internal/credential"
	"boAIrd/internal/provider"
	"boAIrd/internal/provider/adapters"
)

type fakeProvider struct{}

func (fakeProvider) Chat(context.Context, provider.ChatRequest) (provider.ChatResponse, error) {
	return provider.ChatResponse{}, nil
}
func (fakeProvider) Models(context.Context) ([]provider.Model, error) { return nil, nil }

func TestRegistryConfiguresAndResolvesAdapters(t *testing.T) {
	registry := provider.NewRegistry()
	if err := adapters.Register(registry); err != nil {
		t.Fatal(err)
	}
	resolver := staticCredential("test-secret")
	if err := registry.Configure("work-openai", "openai", provider.Config{CredentialID: "credential-1", CredentialResolver: resolver}); err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve("work-openai")
	if err != nil {
		t.Fatal(err)
	}
	if resolved == nil {
		t.Fatal("resolved provider is nil")
	}
	if err := registry.Configure("compatible", "openai-compatible", provider.Config{CredentialID: "credential-2", CredentialResolver: resolver}); !errors.Is(err, provider.ErrInvalidConfiguration) {
		t.Fatalf("expected explicit base URL requirement, got %v", err)
	}
	if _, err := registry.Resolve("missing"); !errors.Is(err, provider.ErrProviderNotFound) {
		t.Fatalf("expected provider not found, got %v", err)
	}
	if got, want := registry.IDs(), []string{"work-openai"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs = %v, want %v", got, want)
	}
}

func TestRegistryAllowsBuiltInProviderDefaults(t *testing.T) {
	registry := provider.NewRegistry()
	if err := adapters.Register(registry); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"openai", "anthropic", "deepseek", "gemini"} {
		t.Run(name, func(t *testing.T) {
			if err := registry.Configure(name, name, provider.Config{CredentialID: "credential", CredentialResolver: staticCredential("secret")}); err != nil {
				t.Fatalf("configure built-in default: %v", err)
			}
		})
	}
}

func TestRegistryRejectsDuplicates(t *testing.T) {
	registry := provider.NewRegistry()
	if err := registry.Register("one", fakeProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("one", fakeProvider{}); !errors.Is(err, provider.ErrProviderAlreadyExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestValidateChatRequest(t *testing.T) {
	valid := provider.ChatRequest{Model: "model", MaxTokens: 10, Messages: []provider.ChatMessage{{Role: provider.RoleSystem, Content: "system"}, {Role: provider.RoleUser, Content: "hello"}}}
	if err := provider.ValidateChatRequest(valid); err != nil {
		t.Fatalf("valid request: %v", err)
	}
	invalid := valid
	invalid.Messages = []provider.ChatMessage{{Role: provider.RoleUser, Content: "hi"}, {Role: provider.RoleSystem, Content: "late"}}
	if err := provider.ValidateChatRequest(invalid); !errors.Is(err, provider.ErrInvalidRequest) {
		t.Fatalf("expected invalid request, got %v", err)
	}
}

type staticCredential string

func (s staticCredential) Resolve(context.Context, string, credential.Scope) (string, error) {
	return string(s), nil
}
