package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/template"
)

func newTemplateRepo(t *testing.T) (*TemplateRepository, *Database) {
	t.Helper()
	ctx := context.Background()
	database, err := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`INSERT INTO projects(id,name,status,human_review_required) VALUES ('p1','Project','CREATED',1)`); err != nil {
		t.Fatal(err)
	}
	repository, err := NewTemplateRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	return repository, database
}

func TestTemplateRepositoryUpdateCAS(t *testing.T) {
	ctx := context.Background()
	repository, _ := newTemplateRepo(t)
	now := time.Now().UTC()
	value := template.Template{
		ID: "t1", Key: "qa", Name: "QA", Role: "qa", Instructions: "review",
		DefaultRuntime: agent.RuntimeLocal, SuggestedTools: []string{}, SuggestedCapabilities: []string{},
		Version: 1, CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.CreateWithAudit(ctx, value, template.AuditEvent{ActorID: "admin", Action: "created", TemplateKey: "qa", Version: 1}); err != nil {
		t.Fatal(err)
	}
	next := value
	next.Version = 2
	next.Name = "QA v2"
	next.UpdatedAt = now
	if err := repository.UpdateWithAudit(ctx, next, 1, template.AuditEvent{ActorID: "admin", Action: "updated", TemplateKey: "qa", Version: 2}); err != nil {
		t.Fatalf("first update: %v", err)
	}
	if err := repository.UpdateWithAudit(ctx, next, 1, template.AuditEvent{ActorID: "admin", Action: "updated", TemplateKey: "qa", Version: 2}); err == nil {
		t.Fatal("expected version conflict on stale update")
	}
}

func TestTemplateRepositoryInstantiateAgentAtomic(t *testing.T) {
	ctx := context.Background()
	repository, database := newTemplateRepo(t)
	agentValue, err := agent.NewConfigured("agent-real", agent.Configuration{
		Name: "Worker", Role: "worker", Runtime: agent.RuntimeLocal, Capabilities: []string{"task.read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The instance references a non-existent Agent ID, so the template_instances
	// insert fails after the Agent and membership inserts succeed in-transaction.
	_, err = repository.InstantiateAgent(ctx, agentValue, template.Instance{
		AgentID: "ghost-agent", ProjectID: "p1", TemplateKey: "qa", TemplateVersion: 1, CreatedAt: time.Now().UTC(),
	}, template.AuditEvent{ActorID: "admin", Action: "instantiated", TemplateKey: "qa", ProjectID: "p1", Version: 1})
	if err == nil {
		t.Fatal("expected instantiation to fail")
	}
	assertCount(t, database, "SELECT COUNT(*) FROM agents WHERE id='agent-real'", 0)
	assertCount(t, database, "SELECT COUNT(*) FROM agent_projects WHERE agent_id='agent-real'", 0)
	assertCount(t, database, "SELECT COUNT(*) FROM template_instances WHERE agent_id='ghost-agent'", 0)
	assertCount(t, database, "SELECT COUNT(*) FROM audit_events WHERE event_type='template.instantiated'", 0)
}

func TestTemplateRepositoryRejectsArchivedProjectInsideTransaction(t *testing.T) {
	ctx := context.Background()
	repository, database := newTemplateRepo(t)
	if _, err := database.ExecContext(ctx, `INSERT INTO projects(id, name, status, human_review_required) VALUES ('p2', 'Archived', 'ARCHIVED', 1)`); err != nil {
		t.Fatal(err)
	}
	agentValue, err := agent.NewConfigured("archived-agent", agent.Configuration{
		Name: "Worker", Role: "worker", Runtime: agent.RuntimeLocal, Capabilities: []string{"task.read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.InstantiateAgent(ctx, agentValue, template.Instance{
		AgentID: "archived-agent", ProjectID: "p2", TemplateKey: "qa", TemplateVersion: 1, CreatedAt: time.Now().UTC(),
	}, template.AuditEvent{ActorID: "admin", Action: "instantiated", TemplateKey: "qa", ProjectID: "p2", Version: 1})
	if !errors.Is(err, template.ErrProjectArchived) {
		t.Fatalf("archived-project instantiation error = %v, want ErrProjectArchived", err)
	}
	assertCount(t, database, "SELECT COUNT(*) FROM agents WHERE id='archived-agent'", 0)
	assertCount(t, database, "SELECT COUNT(*) FROM agent_projects WHERE agent_id='archived-agent'", 0)
	assertCount(t, database, "SELECT COUNT(*) FROM template_instances WHERE agent_id='archived-agent'", 0)
	assertCount(t, database, "SELECT COUNT(*) FROM audit_events WHERE resource_id='archived-agent'", 0)
}

func assertCount(t *testing.T, database *Database, query string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("%q = %d, want %d", query, got, want)
	}
}
