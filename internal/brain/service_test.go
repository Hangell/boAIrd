package brain_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/credential"
	"boAIrd/internal/project"
	"boAIrd/internal/storage"
)

func TestServiceCreatesAndExecutesBrainWithPersistedRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("provider path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Errorf("authorization header missing or incorrect")
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode provider request: %v", err)
		}
		_, _ = w.Write([]byte(`{"model":"test-model","choices":[{"message":{"content":"A proposed plan."},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	projectRepository, _ := storage.NewProjectRepository(database)
	projectManager, _ := project.NewManager(projectRepository)
	credentialRepository, _ := storage.NewCredentialRepository(database)
	credentialManager, err := credential.NewManager(credentialRepository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := credentialManager.Create(ctx, "test", "openai-compatible", "provider-secret")
	if err != nil {
		t.Fatal(err)
	}
	projects, err := projectManager.Create(ctx, project.CreateInput{Name: "Test project"})
	if err != nil {
		t.Fatal(err)
	}
	agentRepository, _ := storage.NewAgentRepository(database)
	agentManager, _ := agent.NewManager(agentRepository)
	configuredAgent, err := agentManager.Create(ctx, agent.CreateInput{
		Name: "Project Brain", Role: "coordinator", Runtime: agent.RuntimeAPI, Provider: "openai-compatible",
		ProviderBaseURL: server.URL + "/v1", Model: "test-model", CredentialID: metadata.ID,
		Capabilities: []string{"brain.execute"},
	})
	if err != nil {
		t.Fatal(err)
	}
	brainRepository, _ := storage.NewBrainRepository(database)
	runRepository, _ := storage.NewRunRepository(database)
	service, err := brain.NewService(brainRepository, projectManager, agentManager, runRepository, credentialManager)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(ctx, projects.ID(), brain.CreateInput{AgentID: configuredAgent.ID(), Autonomy: brain.AutonomyAutonomous})
	if err != nil {
		t.Fatal(err)
	}
	if created.Autonomy() != brain.AutonomyAutonomous {
		t.Fatalf("autonomy = %s", created.Autonomy())
	}
	result, err := service.Execute(ctx, projects.ID(), "Plan a small feature", 64)
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Run.Status()) != "SUCCESS" {
		t.Fatalf("run status = %s", result.Run.Status())
	}
	if result.Content != "A proposed plan." {
		t.Fatalf("content = %q", result.Content)
	}
	var status, storedOutput string
	if err := database.QueryRowContext(ctx, "SELECT status, output FROM runs WHERE id = ?", result.Run.ID()).Scan(&status, &storedOutput); err != nil {
		t.Fatal(err)
	}
	if status != "SUCCESS" || storedOutput != result.Content {
		t.Fatalf("persisted run = %s, %q", status, storedOutput)
	}
	var storedInput string
	if err := database.QueryRowContext(ctx, "SELECT input FROM runs WHERE id = ?", result.Run.ID()).Scan(&storedInput); err != nil {
		t.Fatal(err)
	}
	if storedInput != "Objective: Plan a small feature" {
		t.Fatalf("stored input = %q", storedInput)
	}
}
