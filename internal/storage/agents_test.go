package storage

import (
	"context"
	"path/filepath"
	"testing"

	"boAIrd/internal/agent"
)

func TestAgentRepositoryPersistsConfigurationAndRelations(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if _, err := database.Exec(`INSERT INTO credentials(id, name, provider, secret_ciphertext) VALUES ('cred-1', 'OpenAI', 'openai', ?)`, make([]byte, 28)); err != nil {
		t.Fatal(err)
	}
	repository, err := NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := agent.NewManager(repository)
	if err != nil {
		t.Fatal(err)
	}
	value, err := manager.Create(context.Background(), agent.CreateInput{
		Name: "Backend Go", Role: "developer", Runtime: agent.RuntimeAPI, Provider: "OpenAI", Model: "gpt-test",
		CredentialID: "cred-1", Instructions: "Implement Go APIs", LanguagePreferences: []string{"pt-BR", "en-US"},
		Capabilities: []string{"filesystem.read", "github.issue.create"},
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := manager.Get(context.Background(), value.ID())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Provider() != "openai" || loaded.Model() != "gpt-test" || loaded.CredentialID() != "cred-1" || loaded.Status() != agent.StatusIdle {
		t.Fatalf("persisted configuration mismatch: %+v", loaded.Snapshot())
	}
	if got := len(loaded.Capabilities()); got != 2 {
		t.Fatalf("capability count = %d, want 2", got)
	}
	languages := loaded.LanguagePreferences()
	if len(languages) != 2 || languages[0] != "pt-BR" || languages[1] != "en-US" {
		t.Fatalf("language preferences = %#v", languages)
	}
	newName := "Backend Go Senior"
	updated, err := manager.Update(context.Background(), value.ID(), agent.UpdateInput{Name: &newName})
	if err != nil || updated.Name() != newName || updated.Provider() != "openai" {
		t.Fatalf("Update() = %+v, %v", updated.Snapshot(), err)
	}
	if got := updated.LanguagePreferences(); len(got) != 2 || got[0] != "pt-BR" || got[1] != "en-US" {
		t.Fatalf("language preferences after update = %#v", got)
	}
	listed, err := manager.List(context.Background(), 0, 0)
	if err != nil || len(listed) != 1 || listed[0].Name() != newName {
		t.Fatalf("List() = %#v, %v", listed, err)
	}
}

func TestAgentRepositoryScopesMemberListsAndDetailsAcrossProjects(t *testing.T) {
	ctx := context.Background()
	database, err := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	for _, project := range []struct{ id, name string }{{"project-a", "Alpha"}, {"project-b", "Beta"}} {
		if _, err := database.ExecContext(ctx, `INSERT INTO projects(id, name, status, human_review_required) VALUES (?, ?, 'CREATED', 0)`, project.id, project.name); err != nil {
			t.Fatal(err)
		}
	}
	for _, membership := range []struct{ projectID, userID string }{{"project-a", "alice"}, {"project-b", "bob"}} {
		if _, err := database.ExecContext(ctx, `INSERT INTO project_members(project_id, user_id, role) VALUES (?, ?, 'MEMBER')`, membership.projectID, membership.userID); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []struct {
		id, name, projectID string
		brain               bool
	}{
		{"agent-a", "Visible agent", "project-a", false},
		{"agent-b", "Foreign agent", "project-b", false},
		{"brain-a", "Visible brain", "project-a", true},
	} {
		if _, err := database.ExecContext(ctx, `INSERT INTO agents(id, name, role, runtime) VALUES (?, ?, 'worker', 'MCP')`, value.id, value.name); err != nil {
			t.Fatal(err)
		}
		if value.brain {
			if _, err := database.ExecContext(ctx, `INSERT INTO agent_capabilities(agent_id, capability) VALUES (?, 'brain.execute')`, value.id); err != nil {
				t.Fatal(err)
			}
			if _, err := database.ExecContext(ctx, `INSERT INTO brains(id, project_id, active) VALUES ('brain-a-id', ?, 1)`, value.projectID); err != nil {
				t.Fatal(err)
			}
			if _, err := database.ExecContext(ctx, `INSERT INTO brain_configuration(brain_id, agent_id, autonomy) VALUES ('brain-a-id', ?, 'MANUAL')`, value.id); err != nil {
				t.Fatal(err)
			}
			if _, err := database.ExecContext(ctx, `UPDATE projects SET brain_id = 'brain-a-id' WHERE id = ?`, value.projectID); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := database.ExecContext(ctx, `INSERT INTO agent_projects(agent_id, project_id) VALUES (?, ?)`, value.id, value.projectID); err != nil {
				t.Fatal(err)
			}
		}
	}
	repository, err := NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	values, err := repository.ListForUser(ctx, "alice", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].ID() != "agent-a" || values[1].ID() != "brain-a" {
		t.Fatalf("Alice's Agents = %#v, want direct Agent and active Brain from project-a", values)
	}
	for agentID, want := range map[string]bool{"agent-a": true, "brain-a": true, "agent-b": false} {
		got, err := repository.HasProjectAssociation(ctx, agentID, "alice")
		if err != nil || got != want {
			t.Fatalf("HasProjectAssociation(%q, alice) = %t, %v; want %t", agentID, got, err, want)
		}
	}
}

func TestAgentRepositoryEnforcesCredentialReferences(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	repository, err := NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := agent.NewManager(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Create(context.Background(), agent.CreateInput{Name: "API agent", Role: "worker", Runtime: agent.RuntimeAPI, Provider: "openai", Model: "gpt-test", CredentialID: "missing"})
	if err != agent.ErrCredentialMismatch {
		t.Fatalf("create with missing credential reference error = %v", err)
	}
	if _, err := database.Exec(`INSERT INTO credentials(id, name, provider, secret_ciphertext) VALUES ('cred-1', 'OpenAI', 'openai', ?)`, make([]byte, 28)); err != nil {
		t.Fatal(err)
	}
	_, err = manager.Create(context.Background(), agent.CreateInput{Name: "Mismatch", Role: "worker", Runtime: agent.RuntimeAPI, Provider: "anthropic", Model: "claude", CredentialID: "cred-1"})
	if err != agent.ErrCredentialMismatch {
		t.Fatalf("create with mismatched provider credential error = %v", err)
	}
}

func TestAgentProjectScopePersistsAndAuditsOnlyRealChanges(t *testing.T) {
	ctx := context.Background()
	database, err := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if _, err := database.ExecContext(ctx, `INSERT INTO projects(id, name, status, human_review_required) VALUES ('project-1', 'Project One', 'CREATED', 0)`); err != nil {
		t.Fatal(err)
	}
	repository, err := NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := agent.NewManager(repository)
	if err != nil {
		t.Fatal(err)
	}
	value, err := manager.Create(ctx, agent.CreateInput{Name: "Scoped agent", Role: "worker", Runtime: agent.RuntimeMCP, Capabilities: []string{"project.read"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.AddProject(ctx, value.ID(), "project-1", "local-administrator"); err != nil {
		t.Fatal(err)
	}
	if err := manager.AddProject(ctx, value.ID(), "project-1", "local-administrator"); err != nil {
		t.Fatal(err)
	}
	loadedRepository, err := NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	loadedManager, err := agent.NewManager(loadedRepository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadedManager.Get(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.ProjectIDs(); len(got) != 1 || got[0] != "project-1" {
		t.Fatalf("persisted project IDs = %#v, want [project-1]", got)
	}
	assertAgentProjectAuditCount(t, ctx, database, "agent.project.joined", 1)
	if _, err := database.ExecContext(ctx, `UPDATE audit_events SET reason = 'tampered' WHERE event_type = 'agent.project.joined'`); err == nil {
		t.Fatal("audit event update was allowed")
	}
	if err := loadedManager.RemoveProject(ctx, value.ID(), "project-1", "local-administrator"); err != nil {
		t.Fatal(err)
	}
	if err := loadedManager.RemoveProject(ctx, value.ID(), "project-1", "local-administrator"); err != nil {
		t.Fatal(err)
	}
	revoked, err := loadedManager.Get(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(revoked.ProjectIDs()) != 0 {
		t.Fatalf("revoked scope remained after reload: %#v", revoked.ProjectIDs())
	}
	assertAgentProjectAuditCount(t, ctx, database, "agent.project.left", 1)
}

func assertAgentProjectAuditCount(t *testing.T, ctx context.Context, database *Database, eventType string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE event_type = ?`, eventType).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("audit count for %q = %d, want %d", eventType, got, want)
	}
}

func TestActiveBrainAgentCapabilitiesAreUpdatedDifferentially(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if _, err := database.Exec(`INSERT INTO credentials(id, name, provider, secret_ciphertext) VALUES ('cred-1', 'OpenAI', 'openai', ?)`, make([]byte, 28)); err != nil {
		t.Fatal(err)
	}
	repository, err := NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := agent.NewManager(repository)
	if err != nil {
		t.Fatal(err)
	}
	value, err := manager.Create(context.Background(), agent.CreateInput{
		Name: "Brain agent", Role: "coordinator", Runtime: agent.RuntimeAPI, Provider: "openai", Model: "gpt-test",
		CredentialID: "cred-1", Capabilities: []string{"brain.execute", "task.read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required) VALUES ('p1', 'Project', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO brains(id, project_id, active) VALUES ('b1', 'p1', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO brain_configuration(brain_id, agent_id, autonomy) VALUES ('b1', ?, 'MANUAL')`, value.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE projects SET brain_id = 'b1' WHERE id = 'p1'`); err != nil {
		t.Fatal(err)
	}

	newName := "Principal Brain"
	updated, err := manager.Update(context.Background(), value.ID(), agent.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("update active Brain agent name: %v", err)
	}
	if updated.Name() != newName {
		t.Fatalf("agent name = %q, want %q", updated.Name(), newName)
	}
	if got := updated.Capabilities(); len(got) != 2 || got[0] != "brain.execute" || got[1] != "task.read" {
		t.Fatalf("capabilities after unrelated update = %#v", got)
	}

	capabilities := []string{"task.read"}
	if _, err := manager.Update(context.Background(), value.ID(), agent.UpdateInput{Capabilities: &capabilities}); err == nil {
		t.Fatal("expected explicit removal of brain.execute from active Brain to fail")
	}
	unchanged, err := manager.Get(context.Background(), value.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got := unchanged.Capabilities(); len(got) != 2 || got[0] != "brain.execute" || got[1] != "task.read" {
		t.Fatalf("capabilities after rejected removal = %#v", got)
	}
}
