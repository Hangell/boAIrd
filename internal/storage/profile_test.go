package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"boAIrd/internal/profile"
)

func TestProfileRepositoryUpsertAndList(t *testing.T) {
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
	repository, err := NewProfileRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Get(ctx, "p1", "qa"); err == nil {
		t.Fatal("expected missing override to return an error")
	}

	created := time.Now().UTC().Add(-time.Minute)
	first := profile.Override{
		ID: "o1", ProjectID: "p1", ProfileKey: "qa", Instructions: "first",
		Capabilities: []string{"task.read"}, Tools: []string{"filesystem"},
		CreatedBy: "admin", UpdatedBy: "admin", CreatedAt: created, UpdatedAt: created,
	}
	if err := repository.Save(ctx, first); err != nil {
		t.Fatal(err)
	}
	got, err := repository.Get(ctx, "p1", "qa")
	if err != nil {
		t.Fatal(err)
	}
	if got.Instructions != "first" || len(got.Capabilities) != 1 || len(got.Tools) != 1 {
		t.Fatalf("override = %+v", got)
	}

	second := first
	second.Instructions = "second"
	second.Capabilities = []string{"task.read", "task.review"}
	second.Tools = []string{}
	second.UpdatedBy = "admin-2"
	second.UpdatedAt = time.Now().UTC()
	if err := repository.Save(ctx, second); err != nil {
		t.Fatal(err)
	}
	updated, err := repository.Get(ctx, "p1", "qa")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Instructions != "second" || len(updated.Capabilities) != 2 || len(updated.Tools) != 0 {
		t.Fatalf("updated override = %+v", updated)
	}
	if !updated.CreatedAt.Equal(created) {
		t.Fatalf("created_at changed on update: %v vs %v", updated.CreatedAt, created)
	}

	values, err := repository.List(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 {
		t.Fatalf("list = %d overrides, want 1", len(values))
	}
}
