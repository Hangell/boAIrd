package extension

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"boAIrd/internal/capability"
)

func validManifest() Manifest {
	return Manifest{
		SDKVersion: 1,
		Name:       "example-tools",
		Version:    "1.0.0",
		Tools: []ToolDeclaration{
			{
				Name: "example.search", Description: "Search an allowlisted API",
				Kind: KindHTTP, Endpoint: "https://api.example.com/search",
				InputSchema:          json.RawMessage(`{"type":"object"}`),
				OutputSchema:         json.RawMessage(`{"type":"object"}`),
				RequiredCapabilities: []capability.Capability{"example.search"},
				DataAccess:           "network:api.example.com",
			},
		},
	}
}

func TestValidateManifestRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Manifest)
		wantErr error
	}{
		{"unsupported sdk", func(m *Manifest) { m.SDKVersion = 2 }, ErrInvalidManifest},
		{"bad extension name", func(m *Manifest) { m.Name = "Bad Name" }, ErrInvalidManifest},
		{"no tools", func(m *Manifest) { m.Tools = nil }, ErrInvalidManifest},
		{"bad tool kind", func(m *Manifest) { m.Tools[0].Kind = ToolKind("shell") }, ErrInvalidManifest},
		{"missing capability", func(m *Manifest) { m.Tools[0].RequiredCapabilities = nil }, ErrInvalidManifest},
		{"invalid capability", func(m *Manifest) { m.Tools[0].RequiredCapabilities = []capability.Capability{"Bad Cap"} }, ErrInvalidManifest},
		{"bad schema", func(m *Manifest) { m.Tools[0].InputSchema = json.RawMessage(`[]`) }, ErrInvalidManifest},
		{"http without endpoint", func(m *Manifest) { m.Tools[0].Endpoint = "" }, ErrInvalidManifest},
		{"http endpoint with credentials", func(m *Manifest) { m.Tools[0].Endpoint = "https://user:pass@api.example.com/x" }, ErrInvalidManifest},
		{"filesystem data access", func(m *Manifest) { m.Tools[0].DataAccess = "filesystem:/tmp" }, ErrInvalidManifest},
		{"secret data access", func(m *Manifest) { m.Tools[0].DataAccess = "secret:credential" }, ErrInvalidManifest},
		{"shell data access", func(m *Manifest) { m.Tools[0].DataAccess = "shell:execute" }, ErrInvalidManifest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validManifest()
			test.mutate(&manifest)
			if err := ValidateManifest(manifest); !errors.Is(err, test.wantErr) {
				t.Fatalf("ValidateManifest() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

type repositoryStub struct {
	extensions map[string]Extension
}

func newRepositoryStub() *repositoryStub { return &repositoryStub{extensions: map[string]Extension{}} }

func (s *repositoryStub) Create(_ context.Context, value Extension) error {
	if _, ok := s.extensions[value.Name()]; ok {
		return ErrAlreadyExists
	}
	s.extensions[value.Name()] = value
	return nil
}

func (s *repositoryStub) List(context.Context) ([]Extension, error) {
	values := make([]Extension, 0, len(s.extensions))
	for _, value := range s.extensions {
		values = append(values, value)
	}
	return values, nil
}

func (s *repositoryStub) Get(_ context.Context, name string) (Extension, error) {
	value, ok := s.extensions[name]
	if !ok {
		return Extension{}, ErrNotFound
	}
	return value, nil
}

func (s *repositoryStub) SetStatus(_ context.Context, name string, status Status) error {
	value, ok := s.extensions[name]
	if !ok {
		return ErrNotFound
	}
	value.status = status
	s.extensions[name] = value
	return nil
}

func (s *repositoryStub) Delete(_ context.Context, name string) error {
	delete(s.extensions, name)
	return nil
}

type registrarStub struct {
	registered   []string
	unregistered []string
}

func (r *registrarStub) RegisterTool(_ context.Context, declaration ToolDeclaration) error {
	r.registered = append(r.registered, declaration.Name)
	return nil
}

func (r *registrarStub) UnregisterTool(_ context.Context, name string) {
	r.unregistered = append(r.unregistered, name)
}

type auditStub struct{ events []string }

func (a *auditStub) RecordExtensionEvent(_ context.Context, _, name, eventType string) error {
	a.events = append(a.events, name+":"+eventType)
	return nil
}

func TestServiceInstallActivateDeactivate(t *testing.T) {
	repository := newRepositoryStub()
	registrar := &registrarStub{}
	audit := &auditStub{}
	service, err := NewService(repository, registrar, audit)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	installed, err := service.Install(ctx, validManifest(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if installed.Status() != StatusPending {
		t.Fatalf("installed status = %s, want pending", installed.Status())
	}
	active, err := service.Activate(ctx, "example-tools", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if active.Status() != StatusActive || len(registrar.registered) != 1 || registrar.registered[0] != "example.search" {
		t.Fatalf("activated = %#v, registered = %v", active, registrar.registered)
	}
	if err := service.Deactivate(ctx, "example-tools", "admin"); err != nil {
		t.Fatal(err)
	}
	if len(registrar.unregistered) != 1 || registrar.unregistered[0] != "example.search" {
		t.Fatalf("unregistered = %v", registrar.unregistered)
	}
	if len(audit.events) != 3 {
		t.Fatalf("audit events = %v, want install/activate/deactivate", audit.events)
	}
}

func TestServiceActivateRequiresPending(t *testing.T) {
	repository := newRepositoryStub()
	service, err := NewService(repository, &registrarStub{}, &auditStub{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Install(context.Background(), validManifest(), "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(context.Background(), "example-tools", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(context.Background(), "example-tools", "admin"); !errors.Is(err, ErrWrongStatus) {
		t.Fatalf("second Activate() error = %v, want wrong status", err)
	}
}
