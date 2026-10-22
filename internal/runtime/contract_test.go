package runtime_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/capability"
	"boAIrd/internal/credential"
	"boAIrd/internal/provider"
	"boAIrd/internal/runtime"
)

type fakeAgentReader struct {
	agents map[string]agent.Agent
}

func (f fakeAgentReader) Get(_ context.Context, id string) (agent.Agent, error) {
	value, ok := f.agents[id]
	if !ok {
		return agent.Agent{}, agent.ErrNotFound
	}
	return value, nil
}

type fakeCredentialResolver struct{}

func (fakeCredentialResolver) Resolve(context.Context, string, credential.Scope) (string, error) {
	return "provider-secret", nil
}

type fakeRuntime struct {
	calls atomic.Int32
}

func (f *fakeRuntime) Execute(context.Context, runtime.Request) (runtime.Result, error) {
	f.calls.Add(1)
	return runtime.Result{Output: "executed"}, nil
}

func (*fakeRuntime) Runtime() agent.Runtime { return agent.RuntimeRemote }
func (*fakeRuntime) RequiredCapabilities() []capability.Capability {
	return []capability.Capability{"task.transition"}
}
func (*fakeRuntime) Status(context.Context, string) (agent.Status, error) {
	return agent.StatusIdle, nil
}

func mustAgent(t *testing.T, id string, kind agent.Runtime, capabilities ...string) agent.Agent {
	t.Helper()
	configuration := agent.Configuration{Name: "Agent", Role: "worker", Runtime: kind, Capabilities: capabilities}
	if kind == agent.RuntimeAPI {
		configuration.Provider = "openai-compatible"
		configuration.Model = "test-model"
		configuration.CredentialID = "cred-1"
	}
	value, err := agent.NewConfigured(id, configuration)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := value.Snapshot()
	snapshot.ProjectIDs = []string{"p1"}
	value, err = agent.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestRegistryResolveAndValidation(t *testing.T) {
	registry := runtime.NewRegistry()
	mcp, err := runtime.NewMCPRuntime(fakeAgentReader{})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(agent.RuntimeMCP, mcp); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve(agent.RuntimeMCP); err != nil {
		t.Fatalf("resolve registered runtime: %v", err)
	}
	if _, err := registry.Resolve(agent.RuntimeAPI); !errors.Is(err, runtime.ErrUnknownRuntime) {
		t.Fatalf("resolve unknown runtime error = %v, want ErrUnknownRuntime", err)
	}
	if err := registry.Register(agent.RuntimeMCP, mcp); err == nil {
		t.Fatal("duplicate registration was accepted")
	}
	api, err := runtime.NewAPIExecutor(fakeAgentReader{}, fakeCredentialResolver{}, provider.EndpointPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(agent.RuntimeMCP, api); err == nil {
		t.Fatal("registration accepted an adapter serving a different kind")
	}
}

func TestUnsupportedAdaptersFailClosedAndReportStatus(t *testing.T) {
	build := map[string]func(fakeAgentReader) (runtime.AgentRuntime, error){
		"mcp":    func(r fakeAgentReader) (runtime.AgentRuntime, error) { return runtime.NewMCPRuntime(r) },
		"local":  func(r fakeAgentReader) (runtime.AgentRuntime, error) { return runtime.NewLocalRuntime(r) },
		"remote": func(r fakeAgentReader) (runtime.AgentRuntime, error) { return runtime.NewRemoteRuntime(r) },
	}
	kinds := map[string]agent.Runtime{"mcp": agent.RuntimeMCP, "local": agent.RuntimeLocal, "remote": agent.RuntimeRemote}
	for name, constructor := range build {
		t.Run(name, func(t *testing.T) {
			reader := fakeAgentReader{agents: map[string]agent.Agent{
				"a1": mustAgent(t, "a1", kinds[name], "task.transition"),
			}}
			adapter, err := constructor(reader)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.Execute(context.Background(), runtime.Request{AgentID: "a1", ProjectID: "p1", Input: "work"}); !errors.Is(err, runtime.ErrUnsupportedRuntime) {
				t.Fatalf("execute error = %v, want ErrUnsupportedRuntime", err)
			}
			status, err := adapter.Status(context.Background(), "a1")
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			if status != agent.StatusIdle {
				t.Fatalf("status = %s, want IDLE", status)
			}
			if _, err := adapter.Status(context.Background(), "missing"); !errors.Is(err, agent.ErrNotFound) {
				t.Fatalf("status error = %v, want agent.ErrNotFound", err)
			}
		})
	}
}

func TestEnforcerAppliesPolicyToEveryAdapter(t *testing.T) {
	enforcer := newTestEnforcer(t, map[string]agent.Agent{
		"authorized":   mustAgent(t, "authorized", agent.RuntimeAPI, "task.transition"),
		"unauthorized": mustAgent(t, "unauthorized", agent.RuntimeAPI),
		"mcp-agent":    mustAgent(t, "mcp-agent", agent.RuntimeMCP, "task.transition"),
	})

	if _, err := enforcer.Execute(context.Background(), runtime.Request{AgentID: "unauthorized", ProjectID: "p1", Input: "work"}); !errors.Is(err, runtime.ErrDenied) {
		t.Fatalf("unauthorized execute error = %v, want ErrDenied", err)
	}
	if _, err := enforcer.Execute(context.Background(), runtime.Request{AgentID: "mcp-agent", ProjectID: "p1", Input: "work"}); !errors.Is(err, runtime.ErrUnsupportedRuntime) {
		t.Fatalf("mcp execute error = %v, want ErrUnsupportedRuntime", err)
	}
	if _, err := enforcer.Execute(context.Background(), runtime.Request{AgentID: "missing", ProjectID: "p1", Input: "work"}); !errors.Is(err, agent.ErrNotFound) {
		t.Fatalf("missing agent error = %v, want agent.ErrNotFound", err)
	}
}

func TestEnforcerChecksProjectScopeBeforeCallingAnyAdapter(t *testing.T) {
	worker := mustAgent(t, "worker", agent.RuntimeRemote, "task.transition")
	snapshot := worker.Snapshot()
	snapshot.ProjectIDs = nil
	worker, err := agent.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &fakeRuntime{}
	registry := runtime.NewRegistry()
	if err := registry.Register(agent.RuntimeRemote, adapter); err != nil {
		t.Fatal(err)
	}
	enforcer, err := runtime.NewEnforcer(registry, fakeAgentReader{agents: map[string]agent.Agent{"worker": worker}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := enforcer.Execute(context.Background(), runtime.Request{AgentID: "worker", ProjectID: "p2", Input: "work"}); !errors.Is(err, runtime.ErrDenied) {
		t.Fatalf("cross-project execute error = %v, want ErrDenied", err)
	}
	if calls := adapter.calls.Load(); calls != 0 {
		t.Fatalf("adapter executions = %d, want 0", calls)
	}
}

func TestEnforcerExecutesAPIAdapter(t *testing.T) {
	t.Run("cross-project denied before provider call", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			_, _ = w.Write([]byte(`{"model":"test-model","choices":[{"message":{"content":"unexpected"},"finish_reason":"stop"}]}`))
		}))
		defer server.Close()
		enforcer := newAPIEnforcer(t, server.URL+"/v1", 0)
		if _, err := enforcer.Execute(context.Background(), runtime.Request{AgentID: "worker", ProjectID: "p2", Input: "work"}); !errors.Is(err, runtime.ErrDenied) {
			t.Fatalf("cross-project execute error = %v, want ErrDenied", err)
		}
		if got := calls.Load(); got != 0 {
			t.Fatalf("provider calls = %d, want 0", got)
		}
	})

	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"model":"test-model","choices":[{"message":{"content":"worker deliverable"},"finish_reason":"stop"}]}`))
		}))
		defer server.Close()
		enforcer := newAPIEnforcer(t, server.URL+"/v1", 0)
		result, err := enforcer.Execute(context.Background(), runtime.Request{AgentID: "worker", ProjectID: "p1", Input: "work"})
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if result.Output != "worker deliverable" {
			t.Fatalf("output = %q", result.Output)
		}
	})

	t.Run("failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("provider diagnostic contains provider-secret"))
		}))
		defer server.Close()
		enforcer := newAPIEnforcer(t, server.URL+"/v1", 0)
		_, err := enforcer.Execute(context.Background(), runtime.Request{AgentID: "worker", ProjectID: "p1", Input: "work"})
		if !errors.Is(err, runtime.ErrFailed) {
			t.Fatalf("error = %v, want ErrFailed", err)
		}
		if strings.Contains(err.Error(), "provider-secret") {
			t.Fatalf("error leaked provider detail: %v", err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-release
		}))
		defer server.Close()
		enforcer := newAPIEnforcer(t, server.URL+"/v1", 0)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := enforcer.Execute(ctx, runtime.Request{AgentID: "worker", ProjectID: "p1", Input: "work"})
			done <- err
		}()
		<-started
		cancel()
		err := <-done
		close(release)
		if !errors.Is(err, runtime.ErrCancelled) || !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want ErrCancelled wrapping context.Canceled", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-release
		}))
		defer server.Close()
		enforcer := newAPIEnforcer(t, server.URL+"/v1", 40*time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err := enforcer.Execute(ctx, runtime.Request{AgentID: "worker", ProjectID: "p1", Input: "work"})
		close(release)
		if !errors.Is(err, runtime.ErrTimeout) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v, want ErrTimeout wrapping context.DeadlineExceeded", err)
		}
	})
}

func newTestEnforcer(t *testing.T, agents map[string]agent.Agent) *runtime.Enforcer {
	t.Helper()
	registry := runtime.NewRegistry()
	api, err := runtime.NewAPIExecutor(fakeAgentReader{}, fakeCredentialResolver{}, provider.EndpointPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(agent.RuntimeAPI, api); err != nil {
		t.Fatal(err)
	}
	mcp, err := runtime.NewMCPRuntime(fakeAgentReader{})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(agent.RuntimeMCP, mcp); err != nil {
		t.Fatal(err)
	}
	enforcer, err := runtime.NewEnforcer(registry, fakeAgentReader{agents: agents}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return enforcer
}

func newAPIEnforcer(t *testing.T, baseURL string, timeout time.Duration) *runtime.Enforcer {
	t.Helper()
	worker, err := agent.NewConfigured("worker", agent.Configuration{
		Name: "Worker", Role: "worker", Runtime: agent.RuntimeAPI, Provider: "openai-compatible",
		ProviderBaseURL: baseURL, Model: "test-model", CredentialID: "cred-1",
		Capabilities: []string{"task.transition"},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := worker.Snapshot()
	snapshot.ProjectIDs = []string{"p1"}
	worker, err = agent.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	reader := fakeAgentReader{agents: map[string]agent.Agent{"worker": worker}}
	registry := runtime.NewRegistry()
	api, err := runtime.NewAPIExecutor(reader, fakeCredentialResolver{}, provider.EndpointPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(agent.RuntimeAPI, api); err != nil {
		t.Fatal(err)
	}
	enforcer, err := runtime.NewEnforcer(registry, reader, timeout)
	if err != nil {
		t.Fatal(err)
	}
	return enforcer
}
