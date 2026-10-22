package brain_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/credential"
	"boAIrd/internal/project"
	"boAIrd/internal/storage"
)

func TestServiceCreatesAndExecutesBrainWithPersistedRun(t *testing.T) {
	cancelRequestStarted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("provider path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Errorf("authorization header missing or incorrect")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("decode provider request: %v", err)
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode provider JSON: %v", err)
		}
		if strings.Contains(string(body), "trigger-provider-error") {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("provider diagnostic includes provider-secret and api_key=upstream-secret"))
			return
		}
		if strings.Contains(string(body), "trigger-cancellation") {
			cancelRequestStarted <- struct{}{}
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte(`{"model":"test-model","choices":[{"message":{"content":"A proposed plan."},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
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
	var attachmentCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE project_id=? AND resource_type='brain' AND resource_id=? AND event_type='brain.attached' AND actor_id=? AND to_status=?`,
		projects.ID(), created.ID(), project.LocalAdministratorID, configuredAgent.ID()).Scan(&attachmentCount); err != nil {
		t.Fatal(err)
	}
	if attachmentCount != 1 {
		t.Fatalf("Brain attachment audit count = %d, want 1", attachmentCount)
	}
	replacementAgent, err := agentManager.Create(ctx, agent.CreateInput{
		Name: "Replacement Brain", Role: "coordinator", Runtime: agent.RuntimeAPI, Provider: "openai-compatible",
		ProviderBaseURL: server.URL + "/v1", Model: "test-model", CredentialID: metadata.ID,
		Capabilities: []string{"brain.execute"},
	})
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := service.Replace(ctx, projects.ID(), brain.CreateInput{AgentID: replacementAgent.ID(), Autonomy: brain.AutonomyManual}, project.LocalAdministratorID, "administrator rotation api_key=must-not-persist")
	if err != nil {
		t.Fatalf("replace Brain: %v", err)
	}
	if replaced.ID() != created.ID() || replaced.AgentID() != replacementAgent.ID() || replaced.Autonomy() != brain.AutonomyManual {
		t.Fatalf("replacement = id:%s agent:%s autonomy:%s", replaced.ID(), replaced.AgentID(), replaced.Autonomy())
	}
	var auditCount int
	var auditReason string
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*), MAX(reason) FROM audit_events WHERE project_id=? AND resource_type='brain' AND resource_id=? AND event_type='brain.replaced' AND actor_id=?`,
		projects.ID(), created.ID(), project.LocalAdministratorID).Scan(&auditCount, &auditReason); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("Brain replacement audit events = %d, want 1", auditCount)
	}
	var autonomyAuditCount int
	var autonomyFrom, autonomyTo string
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*), MAX(from_status), MAX(to_status) FROM audit_events WHERE project_id=? AND resource_type='brain' AND resource_id=? AND event_type='brain.autonomy.changed' AND actor_id=?`,
		projects.ID(), created.ID(), project.LocalAdministratorID).Scan(&autonomyAuditCount, &autonomyFrom, &autonomyTo); err != nil {
		t.Fatal(err)
	}
	if autonomyAuditCount != 1 || autonomyFrom != string(brain.AutonomyAutonomous) || autonomyTo != string(brain.AutonomyManual) {
		t.Fatalf("autonomy audit = count:%d from:%s to:%s", autonomyAuditCount, autonomyFrom, autonomyTo)
	}
	if strings.Contains(auditReason, "must-not-persist") {
		t.Fatalf("Brain replacement audit reason leaked a secret: %q", auditReason)
	}
	if _, err := service.Replace(ctx, projects.ID(), brain.CreateInput{AgentID: replacementAgent.ID(), Autonomy: brain.AutonomyManual}, project.LocalAdministratorID, "no-op"); err == nil {
		t.Fatal("expected an unchanged Brain replacement to be rejected")
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
	storedRun, err := runRepository.Get(ctx, result.Run.ID())
	if err != nil {
		t.Fatal(err)
	}
	if storedRun.FinishedAt().IsZero() || storedRun.Status() != result.Run.Status() {
		t.Fatalf("retrieved run = %+v", storedRun.Snapshot())
	}
	listed, err := runRepository.List(ctx, projects.ID(), "", replacementAgent.ID(), 10, 0)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list runs = %d, %v", len(listed), err)
	}
	auditEvents, err := runRepository.ListAuditEvents(ctx, result.Run.ID())
	if err != nil || len(auditEvents) != 3 {
		t.Fatalf("successful run audit events = %d, %v", len(auditEvents), err)
	}
	var storedInput string
	if err := database.QueryRowContext(ctx, "SELECT input FROM runs WHERE id = ?", result.Run.ID()).Scan(&storedInput); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(storedInput, "System message:") || !strings.Contains(storedInput, "Plan a small feature") {
		t.Fatalf("stored input = %q", storedInput)
	}

	failed, err := service.Execute(ctx, projects.ID(), "trigger-provider-error api_key=should-not-persist", 64)
	if err == nil || string(failed.Run.Status()) != "FAILED" {
		t.Fatalf("failed execution = %s, %v", failed.Run.Status(), err)
	}
	if failed.Run.Error() != "provider request failed" {
		t.Fatalf("stored provider error = %q", failed.Run.Error())
	}
	if strings.Contains(failed.Run.Input(), "should-not-persist") {
		t.Fatalf("stored input leaked a secret: %q", failed.Run.Input())
	}
	if err := database.QueryRowContext(ctx, "SELECT input FROM runs WHERE id = ?", failed.Run.ID()).Scan(&storedInput); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storedInput, "should-not-persist") {
		t.Fatalf("database input leaked a secret: %q", storedInput)
	}
	var eventCount int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE resource_type='run' AND resource_id=?", failed.Run.ID()).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 3 {
		t.Fatalf("failed run audit events = %d, want created, running, failed", eventCount)
	}
	if _, err := database.ExecContext(ctx, "UPDATE audit_events SET to_status='SUCCESS' WHERE resource_id=?", failed.Run.ID()); err == nil {
		t.Fatal("expected audit updates to be rejected")
	}
	if _, err := database.ExecContext(ctx, "DELETE FROM audit_events WHERE resource_id=?", failed.Run.ID()); err == nil {
		t.Fatal("expected audit events to be immutable")
	}

	callCtx, cancel := context.WithCancel(ctx)
	completed := make(chan struct {
		result brain.Execution
		err    error
	}, 1)
	go func() {
		result, err := service.Execute(callCtx, projects.ID(), "trigger-cancellation", 64)
		completed <- struct {
			result brain.Execution
			err    error
		}{result: result, err: err}
	}()
	select {
	case <-cancelRequestStarted:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("provider request did not start")
	}
	cancel()
	call := <-completed
	if call.err == nil || string(call.result.Run.Status()) != "CANCELLED" {
		t.Fatalf("cancelled execution = %s, %v", call.result.Run.Status(), call.err)
	}
	var cancelledStatus string
	if err := database.QueryRowContext(ctx, "SELECT status FROM runs WHERE id=?", call.result.Run.ID()).Scan(&cancelledStatus); err != nil {
		t.Fatal(err)
	}
	if cancelledStatus != "CANCELLED" || call.result.Run.FinishedAt().IsZero() {
		t.Fatalf("persisted cancellation = %s, %+v", cancelledStatus, call.result.Run.Snapshot())
	}

	externalAgent, err := agentManager.Create(ctx, agent.CreateInput{
		Name: "External Brain", Role: "coordinator", Runtime: agent.RuntimeMCP,
		Capabilities: []string{"brain.execute"},
	})
	if err != nil {
		t.Fatal(err)
	}
	externalProject, err := projectManager.Create(ctx, project.CreateInput{Name: "MCP Brain project"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, externalProject.ID(), brain.CreateInput{AgentID: externalAgent.ID(), Autonomy: brain.AutonomySemiAuto}); err != nil {
		t.Fatalf("create project with MCP Brain: %v", err)
	}
	if _, err := service.Replace(ctx, projects.ID(), brain.CreateInput{AgentID: externalAgent.ID(), Autonomy: brain.AutonomySemiAuto}, project.LocalAdministratorID, "move Brain to authenticated MCP runtime"); err != nil {
		t.Fatalf("replace Brain with MCP agent: %v", err)
	}
	if _, err := service.Execute(ctx, projects.ID(), "must not call a provider", 64); !errors.Is(err, brain.ErrExternalRuntime) {
		t.Fatalf("Execute() error = %v, want ErrExternalRuntime", err)
	}
	var totalRuns int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM runs WHERE project_id = ?", projects.ID()).Scan(&totalRuns); err != nil {
		t.Fatal(err)
	}
	if totalRuns != 3 {
		t.Fatalf("runs after external Brain request = %d, want only the three API runs", totalRuns)
	}
}
