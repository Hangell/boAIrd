package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRunRepositoryListsAuditEventsInOneBatch(t *testing.T) {
	ctx := context.Background()
	database, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required) VALUES ('p1', 'Project', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO agents(id, name, role, runtime) VALUES ('a1', 'Worker', 'worker', 'LOCAL')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"r1", "r2"} {
		if _, err := database.Exec(`INSERT INTO runs(id, project_id, agent_id, status, runtime, provider, model, input)
			VALUES (?, 'p1', 'a1', 'PENDING', 'LOCAL', '', '', '')`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(`UPDATE runs SET status = 'RUNNING' WHERE id = 'r1'`); err != nil {
		t.Fatal(err)
	}
	repository, err := NewRunRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	events, err := repository.ListAuditEventsForRuns(ctx, []string{"r1", "r2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events["r1"]) != 2 || events["r1"][0].EventType != "run.created" || events["r1"][1].EventType != "run.status.changed" {
		t.Fatalf("run r1 events = %#v", events["r1"])
	}
	if len(events["r2"]) != 1 || events["r2"][0].EventType != "run.created" {
		t.Fatalf("run r2 events = %#v", events["r2"])
	}
}
