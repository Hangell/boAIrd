package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestProjectExecutionRequiresExactlyOneActiveBrain(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required)
		VALUES ('p1', 'Project', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE projects SET status = 'READY' WHERE id = 'p1'`); err == nil {
		t.Fatal("expected executable project without a Brain to be rejected")
	}
	if _, err := database.Exec(`INSERT INTO credentials(id, name, provider, secret_ciphertext) VALUES (?, ?, ?, ?)`, "c1", "test", "openai-compatible", make([]byte, 28)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO agents(id,name,role,runtime,provider,model,credential_id) VALUES ('a1','Brain','coordinator','API','openai-compatible','fake','c1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO agent_capabilities(agent_id,capability) VALUES ('a1','brain.execute')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO brains(id, project_id, active) VALUES ('b1', 'p1', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO brain_configuration(brain_id,agent_id,autonomy) VALUES ('b1','a1','MANUAL')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE projects SET brain_id = 'b1', status = 'READY' WHERE id = 'p1'`); err != nil {
		t.Fatalf("set project ready with active Brain: %v", err)
	}
	if _, err := database.Exec(`UPDATE brains SET active = 0 WHERE id = 'b1'`); err == nil {
		t.Fatal("expected active Brain of executable project to remain active")
	}
	if _, err := database.Exec(`DELETE FROM agent_capabilities WHERE agent_id='a1' AND capability='brain.execute'`); err == nil {
		t.Fatal("expected active Brain agent to retain brain.execute")
	}
	if _, err := database.Exec(`INSERT INTO brains(id, project_id, active) VALUES ('b2', 'p1', 1)`); err == nil {
		t.Fatal("expected a project to have at most one active Brain")
	}
}

func TestProjectRepositoryListForUserScopesBeforePagination(t *testing.T) {
	ctx := context.Background()
	database, err := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	for _, project := range []struct{ id, name string }{
		{"project-a", "Alpha"}, {"project-private", "Bravo"}, {"project-c", "Charlie"},
	} {
		if _, err := database.ExecContext(ctx, `INSERT INTO projects(id, name, status, human_review_required) VALUES (?, ?, 'CREATED', 0)`, project.id, project.name); err != nil {
			t.Fatal(err)
		}
	}
	for _, projectID := range []string{"project-a", "project-c"} {
		if _, err := database.ExecContext(ctx, `INSERT INTO project_members(project_id, user_id, role) VALUES (?, 'alice', 'MEMBER')`, projectID); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	values, err := repository.ListForUser(ctx, "alice", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].ID() != "project-c" {
		t.Fatalf("second scoped page = %#v, want only project-c", values)
	}
	values, err = repository.ListForUser(ctx, "alice", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].ID() != "project-a" || values[1].ID() != "project-c" {
		t.Fatalf("scoped projects = %#v, want project-a and project-c", values)
	}
}

func TestProjectExecutionAcceptsMCPBrain(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	statements := []string{
		`INSERT INTO projects(id, name, status, human_review_required) VALUES ('p1', 'Project', 'CREATED', 1)`,
		`INSERT INTO agents(id, name, role, runtime) VALUES ('a1', 'External Brain', 'coordinator', 'MCP')`,
		`INSERT INTO agent_capabilities(agent_id, capability) VALUES ('a1', 'brain.execute')`,
		`INSERT INTO brains(id, project_id, active) VALUES ('b1', 'p1', 1)`,
		`INSERT INTO brain_configuration(brain_id, agent_id, autonomy) VALUES ('b1', 'a1', 'SEMI_AUTO')`,
		`UPDATE projects SET brain_id = 'b1', status = 'READY' WHERE id = 'p1'`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			t.Fatalf("execute %q: %v", statement, err)
		}
	}
	if _, err := database.Exec(`UPDATE agents SET runtime='LOCAL' WHERE id='a1'`); err == nil {
		t.Fatal("expected active Brain to reject a runtime without authenticated external support")
	}
}
