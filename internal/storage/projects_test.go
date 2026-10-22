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
	if _, err := database.Exec(`INSERT INTO brains(id, project_id, active) VALUES ('b1', 'p1', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE projects SET brain_id = 'b1', status = 'READY' WHERE id = 'p1'`); err != nil {
		t.Fatalf("set project ready with active Brain: %v", err)
	}
	if _, err := database.Exec(`UPDATE brains SET active = 0 WHERE id = 'b1'`); err == nil {
		t.Fatal("expected active Brain of executable project to remain active")
	}
	if _, err := database.Exec(`INSERT INTO brains(id, project_id, active) VALUES ('b2', 'p1', 1)`); err == nil {
		t.Fatal("expected a project to have at most one active Brain")
	}
}
