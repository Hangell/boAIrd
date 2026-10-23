package tool

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"boAIrd/internal/capability"
)

type memoryAuditor struct {
	mu     sync.Mutex
	events []Event
	err    error
}

func (a *memoryAuditor) RecordToolExecution(_ context.Context, event Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, event)
	return a.err
}

func (a *memoryAuditor) snapshot() []Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Event(nil), a.events...)
}

type input struct {
	Value string `json:"value"`
}

type output struct {
	Value string `json:"value"`
}

func testTool(t *testing.T, run func(context.Context, input) (output, error)) Definition[input, output] {
	t.Helper()
	return Definition[input, output]{
		Metadata: Contract{
			Name: "test.echo", Description: "Echo validated data",
			InputSchema:          json.RawMessage(`{"type":"object","required":["value"]}`),
			OutputSchema:         json.RawMessage(`{"type":"object","required":["value"]}`),
			RequiredCapabilities: []capability.Capability{"test.execute"},
			Limits:               Limits{Timeout: 100 * time.Millisecond, MaxInputBytes: 128, MaxOutputBytes: 128},
		},
		Validate: func(value input) error {
			if value.Value == "" {
				return errors.New("value is required")
			}
			return nil
		},
		ValidateOutput: func(output) error { return nil },
		Run:            run,
	}
}

func authorizedRequest(raw string) Request {
	return Request{
		ToolName: "test.echo", ProjectID: "p1", ResourceType: "task", ResourceID: "t1",
		Principal: Principal{ID: "agent-1", Type: "agent", Capabilities: []capability.Capability{"test.execute"}, ProjectIDs: []string{"p1"}},
		Input:     json.RawMessage(raw),
	}
}

func setup(t *testing.T, value Tool, auditor *memoryAuditor) *Enforcer {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register(value); err != nil {
		t.Fatal(err)
	}
	scope := ScopeAuthorizerFunc(func(_ context.Context, principal Principal, request Request) error {
		for _, projectID := range principal.ProjectIDs {
			if projectID == request.ProjectID && request.ResourceType == "task" && request.ResourceID == "t1" {
				return nil
			}
		}
		return ErrDenied
	})
	enforcer, err := NewEnforcer(registry, auditor, scope)
	if err != nil {
		t.Fatal(err)
	}
	return enforcer
}

func TestRegistryExecutesAuthorizedTypedToolAndAuditsMetadataOnly(t *testing.T) {
	auditor := &memoryAuditor{}
	enforcer := setup(t, testTool(t, func(_ context.Context, value input) (output, error) {
		return output(value), nil
	}), auditor)
	got, err := enforcer.Execute(context.Background(), authorizedRequest(`{"value":"hello"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(got) != `{"value":"hello"}` {
		t.Fatalf("output = %s", got)
	}
	events := auditor.snapshot()
	if len(events) != 2 || events[0].Outcome != "start" || events[1].Outcome != "success" || events[0].ToolName != "test.echo" {
		t.Fatalf("audit events = %#v", events)
	}
	for _, event := range events {
		if string(got) == event.ResourceID {
			t.Fatal("audit event unexpectedly contains tool output")
		}
	}
}

func TestRegistryRejectsMalformedInputBeforeExecution(t *testing.T) {
	var calls int
	auditor := &memoryAuditor{}
	enforcer := setup(t, testTool(t, func(context.Context, input) (output, error) {
		calls++
		return output{}, nil
	}), auditor)
	if _, err := enforcer.Execute(context.Background(), authorizedRequest(`{"value":"x","unexpected":true}`)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Execute() error = %v, want ErrInvalidInput", err)
	}
	if calls != 0 {
		t.Fatalf("tool executed %d times", calls)
	}
	if events := auditor.snapshot(); len(events) != 1 || events[0].Outcome != "invalid" {
		t.Fatalf("audit events = %#v", events)
	}
}

func TestRegistryDeniesMissingCapability(t *testing.T) {
	var calls int
	auditor := &memoryAuditor{}
	enforcer := setup(t, testTool(t, func(context.Context, input) (output, error) {
		calls++
		return output{}, nil
	}), auditor)
	request := authorizedRequest(`{"value":"x"}`)
	request.Principal.Capabilities = nil
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrDenied) {
		t.Fatalf("Execute() error = %v, want ErrDenied", err)
	}
	if calls != 0 {
		t.Fatalf("tool executed %d times", calls)
	}
}

func TestRegistryDeniesProjectResourceEscapeBeforeExecution(t *testing.T) {
	var calls int
	auditor := &memoryAuditor{}
	enforcer := setup(t, testTool(t, func(context.Context, input) (output, error) {
		calls++
		return output{}, nil
	}), auditor)
	request := authorizedRequest(`{"value":"x"}`)
	request.ResourceID = "outside-task"
	request.Principal.ProjectIDs = nil
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrDenied) {
		t.Fatalf("Execute() error = %v, want ErrDenied", err)
	}
	if calls != 0 {
		t.Fatalf("tool executed %d times", calls)
	}
}

func TestEnforcerRejectsEmptyProjectIDWithPermissiveScopeAuthorizer(t *testing.T) {
	var toolCalls, scopeCalls int
	registry := NewRegistry()
	if err := registry.Register(testTool(t, func(context.Context, input) (output, error) {
		toolCalls++
		return output{Value: "unexpected"}, nil
	})); err != nil {
		t.Fatal(err)
	}
	enforcer, err := NewEnforcer(registry, &memoryAuditor{}, ScopeAuthorizerFunc(func(context.Context, Principal, Request) error {
		scopeCalls++
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	request := authorizedRequest(`{"value":"x"}`)
	request.ProjectID = ""
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Execute() error = %v, want ErrInvalidRequest", err)
	}
	if scopeCalls != 0 || toolCalls != 0 {
		t.Fatalf("scope authorizer calls = %d, tool calls = %d; want both zero", scopeCalls, toolCalls)
	}
}

func TestRegistryBoundsExecutionAndOutputAndFailsOnAuditFailure(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		auditor := &memoryAuditor{}
		enforcer := setup(t, testTool(t, func(ctx context.Context, _ input) (output, error) {
			<-ctx.Done()
			return output{}, ctx.Err()
		}), auditor)
		if _, err := enforcer.Execute(context.Background(), authorizedRequest(`{"value":"x"}`)); !errors.Is(err, ErrTimedOut) {
			t.Fatalf("Execute() error = %v, want ErrTimedOut", err)
		}
		if events := auditor.snapshot(); len(events) != 2 || events[0].Outcome != "start" || events[1].Outcome != "timeout" {
			t.Fatalf("audit events = %#v", events)
		}
	})
	t.Run("tool returns after deadline", func(t *testing.T) {
		auditor := &memoryAuditor{}
		enforcer := setup(t, testTool(t, func(context.Context, input) (output, error) {
			time.Sleep(150 * time.Millisecond)
			return output{Value: "late"}, nil
		}), auditor)
		if _, err := enforcer.Execute(context.Background(), authorizedRequest(`{"value":"x"}`)); !errors.Is(err, ErrTimedOut) {
			t.Fatalf("Execute() error = %v, want ErrTimedOut", err)
		}
	})
	t.Run("output limit", func(t *testing.T) {
		auditor := &memoryAuditor{}
		enforcer := setup(t, testTool(t, func(context.Context, input) (output, error) {
			return output{Value: string(make([]byte, 256))}, nil
		}), auditor)
		if _, err := enforcer.Execute(context.Background(), authorizedRequest(`{"value":"x"}`)); !errors.Is(err, ErrExecution) {
			t.Fatalf("Execute() error = %v, want ErrExecution", err)
		}
	})
	t.Run("invalid typed output", func(t *testing.T) {
		auditor := &memoryAuditor{}
		value := testTool(t, func(context.Context, input) (output, error) {
			return output{}, nil
		})
		value.ValidateOutput = func(value output) error {
			if value.Value == "" {
				return errors.New("value is required")
			}
			return nil
		}
		enforcer := setup(t, value, auditor)
		if _, err := enforcer.Execute(context.Background(), authorizedRequest(`{"value":"x"}`)); !errors.Is(err, ErrExecution) {
			t.Fatalf("Execute() error = %v, want ErrExecution", err)
		}
	})
	t.Run("audit failure", func(t *testing.T) {
		auditor := &memoryAuditor{err: errors.New("database detail")}
		var calls int
		enforcer := setup(t, testTool(t, func(context.Context, input) (output, error) {
			calls++
			return output{Value: "ok"}, nil
		}), auditor)
		if _, err := enforcer.Execute(context.Background(), authorizedRequest(`{"value":"x"}`)); !errors.Is(err, ErrAudit) {
			t.Fatalf("Execute() error = %v, want ErrAudit", err)
		}
		if calls != 0 {
			t.Fatalf("tool executed %d times despite pre-execution audit failure", calls)
		}
	})
}

func TestRegistryRequiresExplicitValidContracts(t *testing.T) {
	registry := NewRegistry()
	value := testTool(t, func(context.Context, input) (output, error) { return output{}, nil })
	value.Metadata.RequiredCapabilities = nil
	if err := registry.Register(value); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Register() error = %v, want ErrInvalidRequest", err)
	}
	value = testTool(t, func(context.Context, input) (output, error) { return output{}, nil })
	if err := registry.Register(value); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(value); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("duplicate Register() error = %v, want ErrInvalidRequest", err)
	}
	if _, err := registry.Resolve("shell.execute"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve() error = %v, want ErrNotFound", err)
	}
}

func TestEnforcerRequiresResourceScopeAuthorizer(t *testing.T) {
	if _, err := NewEnforcer(NewRegistry(), &memoryAuditor{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("NewEnforcer() error = %v, want ErrInvalidRequest", err)
	}
	if _, err := NewEnforcer(NewRegistry(), &memoryAuditor{}, ScopeAuthorizerFunc(nil)); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("NewEnforcer() with nil scope error = %v, want ErrInvalidRequest", err)
	}
}
