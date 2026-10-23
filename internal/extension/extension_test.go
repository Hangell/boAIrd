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
		{"malformed tool name", func(m *Manifest) { m.Tools[0].Name = "example..search" }, ErrInvalidManifest},
		{"missing capability", func(m *Manifest) { m.Tools[0].RequiredCapabilities = nil }, ErrInvalidManifest},
		{"invalid capability", func(m *Manifest) { m.Tools[0].RequiredCapabilities = []capability.Capability{"Bad Cap"} }, ErrInvalidManifest},
		{"bad schema", func(m *Manifest) { m.Tools[0].InputSchema = json.RawMessage(`[]`) }, ErrInvalidManifest},
		{"http without endpoint", func(m *Manifest) { m.Tools[0].Endpoint = "" }, ErrInvalidManifest},
		{"http endpoint with credentials", func(m *Manifest) { m.Tools[0].Endpoint = "https://user:pass@api.example.com/x" }, ErrInvalidManifest},
		{"filesystem data access", func(m *Manifest) { m.Tools[0].DataAccess = "filesystem:/tmp" }, ErrInvalidManifest},
		{"secret data access", func(m *Manifest) { m.Tools[0].DataAccess = "secret:credential" }, ErrInvalidManifest},
		{"shell data access", func(m *Manifest) { m.Tools[0].DataAccess = "shell:execute" }, ErrInvalidManifest},
		{"network host mismatch", func(m *Manifest) { m.Tools[0].DataAccess = "network:other.example.com" }, ErrInvalidManifest},
		{"duplicate tool name", func(m *Manifest) { m.Tools = append(m.Tools, m.Tools[0]) }, ErrInvalidManifest},
		{"unsupported schema keyword", func(m *Manifest) {
			m.Tools[0].InputSchema = json.RawMessage(`{"type":"object","$ref":"https://example.invalid/schema"}`)
		}, ErrInvalidManifest},
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
	events     []string
}

func newRepositoryStub() *repositoryStub { return &repositoryStub{extensions: map[string]Extension{}} }

func (s *repositoryStub) InstallWithAudit(_ context.Context, value Extension, actorID string) error {
	if actorID == "" {
		return errors.New("actor required")
	}
	if _, ok := s.extensions[value.Name()]; ok {
		return ErrAlreadyExists
	}
	s.extensions[value.Name()] = value
	s.events = append(s.events, value.Name()+":extension.installed")
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

func (s *repositoryStub) IsActiveTool(_ context.Context, name string) (bool, error) {
	for _, value := range s.extensions {
		if value.Status() != StatusActive {
			continue
		}
		for _, declaration := range value.Tools() {
			if declaration.Name == name {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *repositoryStub) ActivateWithAudit(_ context.Context, name, actorID string) error {
	value, ok := s.extensions[name]
	if !ok {
		return ErrNotFound
	}
	if value.status != StatusPending {
		return ErrWrongStatus
	}
	value.status = StatusActive
	s.extensions[name] = value
	s.events = append(s.events, name+":extension.activated")
	return nil
}

func (s *repositoryStub) DeactivateWithAudit(_ context.Context, name, actorID string) error {
	value, ok := s.extensions[name]
	if !ok {
		return ErrNotFound
	}
	if value.status != StatusActive {
		return ErrWrongStatus
	}
	delete(s.extensions, name)
	s.events = append(s.events, name+":extension.deactivated")
	return nil
}

type registrarStub struct {
	registered   []string
	unregistered []string
	registerErr  error
}

func (r *registrarStub) RegisterTools(_ context.Context, declarations []ToolDeclaration) error {
	if r.registerErr != nil {
		return r.registerErr
	}
	for _, declaration := range declarations {
		r.registered = append(r.registered, declaration.Name)
	}
	return nil
}

func (r *registrarStub) UnregisterTools(_ context.Context, names []string) {
	r.unregistered = append(r.unregistered, names...)
}

func TestServiceInstallActivateDeactivate(t *testing.T) {
	repository := newRepositoryStub()
	registrar := &registrarStub{}
	service, err := NewService(repository, registrar)
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
	if len(repository.events) != 3 {
		t.Fatalf("audit events = %v, want install/activate/deactivate", repository.events)
	}
}

func TestServiceActivateRequiresPending(t *testing.T) {
	repository := newRepositoryStub()
	service, err := NewService(repository, &registrarStub{})
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

func TestServiceActivationRegistrationFailureRollsBack(t *testing.T) {
	repository := newRepositoryStub()
	registrar := &registrarStub{registerErr: errors.New("registry conflict")}
	service, err := NewService(repository, registrar)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Install(context.Background(), validManifest(), "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(context.Background(), "example-tools", "admin"); err == nil {
		t.Fatal("Activate() unexpectedly succeeded")
	}
	stored, err := repository.Get(context.Background(), "example-tools")
	if err != nil || stored.Status() != StatusPending {
		t.Fatalf("status after registration failure = %s, error = %v; want pending", stored.Status(), err)
	}
	if len(registrar.unregistered) != 0 || len(repository.events) != 1 || repository.events[0] != "example-tools:extension.installed" {
		t.Fatalf("rollback evidence mismatch: unregistered=%v events=%v", registrar.unregistered, repository.events)
	}
}

func TestValidateJSONSchemaEnforcesTypesBoundsAndSingleValue(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","minLength":2},"limit":{"type":"integer","minimum":1,"maximum":10}},"required":["query"],"additionalProperties":false}`)
	for _, value := range []json.RawMessage{
		json.RawMessage(`{"query":"ok","limit":5}`),
		json.RawMessage(`{"query":"valid"}`),
	} {
		if err := ValidateJSONSchema(schema, value); err != nil {
			t.Fatalf("valid value %s rejected: %v", value, err)
		}
	}
	for _, value := range []json.RawMessage{
		json.RawMessage(`{"query":"x"}`),
		json.RawMessage(`{"query":"ok","extra":true}`),
		json.RawMessage(`{"query":"ok","limit":1.5}`),
		json.RawMessage(`{"query":"ok"} {"query":"again"}`),
	} {
		if err := ValidateJSONSchema(schema, value); err == nil {
			t.Fatalf("invalid value %s unexpectedly passed validation", value)
		}
	}
}
