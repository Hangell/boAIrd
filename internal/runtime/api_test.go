package runtime_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/credential"
	"boAIrd/internal/project"
	"boAIrd/internal/provider"
	"boAIrd/internal/runtime"
	"boAIrd/internal/storage"
)

func TestAPIExecutorRunsConfiguredAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("provider path = %q", r.URL.Path)
		}
		if !strings.Contains(r.Header.Get("Authorization"), "Bearer") {
			t.Errorf("provider credential was not resolved")
		}
		_, _ = w.Write([]byte(`{"model":"test-model","choices":[{"message":{"content":"worker deliverable"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	credentialRepository, err := storage.NewCredentialRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	credentialManager, err := credential.NewManager(credentialRepository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := credentialManager.Create(ctx, "worker", "openai-compatible", "provider-secret")
	if err != nil {
		t.Fatal(err)
	}
	projectRepository, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projectManager, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	workProject, err := projectManager.Create(ctx, project.CreateInput{Name: "Runtime"})
	if err != nil {
		t.Fatal(err)
	}
	agentRepository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	agentManager, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := agentManager.Create(ctx, agent.CreateInput{
		Name: "Worker", Role: "backend", Runtime: agent.RuntimeAPI, Provider: "openai-compatible",
		ProviderBaseURL: server.URL + "/v1", Model: "test-model", CredentialID: metadata.ID,
		Capabilities: []string{"task.transition"}, Instructions: "Keep the diff small.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := agentManager.AddProject(ctx, worker.ID(), workProject.ID(), project.LocalAdministratorID); err != nil {
		t.Fatal(err)
	}
	executor, err := runtime.NewAPIExecutor(agentManager, credentialManager, provider.EndpointPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(ctx, runtime.Request{RunID: "run-1", ProjectID: workProject.ID(), TaskID: "task-1", AgentID: worker.ID(), Input: "Do the work"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Output != "worker deliverable" || result.Provider != "openai-compatible" || result.Model != "test-model" {
		t.Fatalf("result = %+v", result)
	}
}

func TestAPIExecutorRejectsNonAPIRuntime(t *testing.T) {
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	agentRepository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	agentManager, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := agentManager.Create(ctx, agent.CreateInput{Name: "External", Role: "backend", Runtime: agent.RuntimeMCP})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := runtime.NewAPIExecutor(agentManager, credentialStub{}, provider.EndpointPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(ctx, runtime.Request{ProjectID: "project-1", AgentID: worker.ID(), Input: "Do the work"}); !errors.Is(err, runtime.ErrUnsupportedRuntime) {
		t.Fatalf("error = %v, want ErrUnsupportedRuntime", err)
	}
}

func TestAPIExecutorRedactsProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("provider diagnostic contains provider-secret"))
	}))
	defer server.Close()

	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	credentialRepository, err := storage.NewCredentialRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	credentialManager, err := credential.NewManager(credentialRepository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := credentialManager.Create(ctx, "worker", "openai-compatible", "provider-secret")
	if err != nil {
		t.Fatal(err)
	}
	projectRepository, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projectManager, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	workProject, err := projectManager.Create(ctx, project.CreateInput{Name: "Runtime"})
	if err != nil {
		t.Fatal(err)
	}
	agentRepository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	agentManager, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := agentManager.Create(ctx, agent.CreateInput{
		Name: "Worker", Role: "backend", Runtime: agent.RuntimeAPI, Provider: "openai-compatible",
		ProviderBaseURL: server.URL + "/v1", Model: "test-model", CredentialID: metadata.ID,
		Capabilities: []string{"task.transition"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := agentManager.AddProject(ctx, worker.ID(), workProject.ID(), project.LocalAdministratorID); err != nil {
		t.Fatal(err)
	}
	executor, err := runtime.NewAPIExecutor(agentManager, credentialManager, provider.EndpointPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = executor.Execute(ctx, runtime.Request{ProjectID: workProject.ID(), AgentID: worker.ID(), Input: "Do the work"})
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if strings.Contains(err.Error(), "provider-secret") {
		t.Fatalf("error leaked provider detail: %v", err)
	}
}

type credentialStub struct{}

func (credentialStub) Resolve(context.Context, string, credential.Scope) (string, error) {
	return "", errors.New("unused")
}
