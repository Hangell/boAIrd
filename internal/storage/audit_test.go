package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/audit"
	"boAIrd/internal/project"
	"boAIrd/internal/tool"
)

func TestAuditRepositoryRecordsImmutableEscalation(t *testing.T) {
	ctx := context.Background()
	database, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	projectRepository, err := NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	p, err := projects.Create(ctx, project.CreateInput{Name: "Escalation"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := NewAuditRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordEscalation(ctx, audit.Escalation{
		ProjectID: p.ID(), TaskID: "task-1", RunID: "run-1", AgentID: "agent-1", Attempts: 3,
		Status: "BLOCKED", Reason: "worker execution failed api_key=must-not-persist",
	}); err != nil {
		t.Fatal(err)
	}
	var actorType, actorID, eventType, resourceType, status, reason string
	if err := database.QueryRowContext(ctx, `SELECT actor_type, actor_id, event_type, resource_type, to_status, reason FROM audit_events WHERE project_id=? AND task_id=?`, p.ID(), "task-1").
		Scan(&actorType, &actorID, &eventType, &resourceType, &status, &reason); err != nil {
		t.Fatal(err)
	}
	if actorType != "system" || actorID != "orchestrator" || eventType != "orchestrator.escalated" || resourceType != "task" || status != "BLOCKED" {
		t.Fatalf("escalation audit = %s/%s/%s/%s/%s", actorType, actorID, eventType, resourceType, status)
	}
	if reason == "" {
		t.Fatal("escalation reason was not recorded")
	}
	if strings.Contains(reason, "must-not-persist") {
		t.Fatalf("escalation reason leaked a secret: %q", reason)
	}
	if _, err := database.ExecContext(ctx, `UPDATE audit_events SET reason='changed' WHERE task_id='task-1'`); err == nil {
		t.Fatal("escalation audit event was mutable")
	}
}

func TestAuditRepositoryRecordsToolExecutionWithoutPayload(t *testing.T) {
	ctx := context.Background()
	database, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "tool-audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	projectRepository, err := NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	p, err := projects.Create(ctx, project.CreateInput{Name: "Tool audit"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := NewAuditRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordToolExecution(ctx, tool.Event{
		ProjectID: p.ID(), ActorType: "agent", ActorID: "agent-1", ToolName: "filesystem.read",
		ResourceType: "file", ResourceID: "file/api_key=secret-value", Outcome: "success",
	}); err != nil {
		t.Fatal(err)
	}
	var actorType, actorID, eventType, resourceType, resourceID, outcome, reason string
	if err := database.QueryRowContext(ctx, `SELECT actor_type, actor_id, event_type, resource_type, resource_id, to_status, reason FROM audit_events WHERE project_id=? AND event_type='tool.executed'`, p.ID()).
		Scan(&actorType, &actorID, &eventType, &resourceType, &resourceID, &outcome, &reason); err != nil {
		t.Fatal(err)
	}
	if actorType != "agent" || actorID != "agent-1" || eventType != "tool.executed" || resourceType != "tool" || resourceID != "filesystem.read" || outcome != "success" {
		t.Fatalf("tool audit = %s/%s/%s/%s/%s/%s", actorType, actorID, eventType, resourceType, resourceID, outcome)
	}
	if strings.Contains(reason, "payload") || strings.Contains(reason, "secret-value") {
		t.Fatalf("tool audit stored caller data: %q", reason)
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM audit_events WHERE event_type='tool.executed'`); err == nil {
		t.Fatal("tool audit event was mutable")
	}
}
