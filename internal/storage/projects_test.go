package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestProjectExecutionRequiresExactlyOneActiveBrain(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required)
		VALUES ('p1', 'Project', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE projects SET status = 'READY' WHERE id = 'p1'`); err == nil {
		t.Fatal("expected executable project without a Brain to be rejected")
	}
	if _, err := database.Exec(`INSERT INTO credentials(id, name, provider, secret_ciphertext) VALUES ('c1', 'test', 'openai-compatible', zeroblob(28))`); err != nil {
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
