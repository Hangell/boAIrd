package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"boAIrd/internal/memory"
)

func TestMemoryRepositoryCRUDAndPrune(t *testing.T) {
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
	repository, err := NewMemoryRepository(database)
	if err != nil {
		t.Fatal(err)
	}

	if version, err := repository.MaxVersion(ctx, "p1", memory.CategoryKnownProblem); err != nil {
		t.Fatal(err)
	} else if version != 0 {
		t.Fatalf("initial max version = %d, want 0", version)
	}

	for i := 1; i <= 3; i++ {
		value, err := memory.New("id-"+string(rune('0'+i)), "p1", memory.CategoryKnownProblem, i, "note", "brain", "brain-1", nil, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if err := repository.Create(ctx, value); err != nil {
			t.Fatal(err)
		}
	}

	values, err := repository.List(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 {
		t.Fatalf("list = %d summaries, want 3", len(values))
	}
	for i, value := range values {
		if value.Version() != i+1 {
			t.Fatalf("list order = %d at index %d, want ascending versions", value.Version(), i)
		}
	}

	deleted, err := repository.DeleteOlderThanVersion(ctx, "p1", memory.CategoryKnownProblem, 2)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	values, err = repository.List(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].Version() != 2 || values[1].Version() != 3 {
		t.Fatalf("after prune list = %d summaries with versions %d,%d, want 2,3", len(values), values[0].Version(), values[1].Version())
	}
}
