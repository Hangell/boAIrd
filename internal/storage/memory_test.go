package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"boAIrd/internal/memory"
)

func TestMemoryRepositoryCRUDAndPrune(t *testing.T) {
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
		if _, err := repository.CreateNext(ctx, value); err != nil {
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

func TestMemoryRepositoryAllocatesVersionsAtomically(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "memory-concurrent.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if _, err := database.ExecContext(ctx, `INSERT INTO projects(id, name, description, status, human_review_required)
		VALUES ('memory-project', 'Memory project', '', 'CREATED', 0)`); err != nil {
		t.Fatal(err)
	}
	repository, err := NewMemoryRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	const writers = 24
	versions := make(chan int, writers)
	errorsFound := make(chan error, writers)
	var waitGroup sync.WaitGroup
	for index := 0; index < writers; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			value, err := memory.New(fmt.Sprintf("memory-%02d", index), "memory-project", memory.CategoryKnownProblem, 1,
				fmt.Sprintf("summary %d", index), "brain", "brain-1", nil, time.Now().UTC())
			if err != nil {
				errorsFound <- err
				return
			}
			created, err := repository.CreateNext(ctx, value)
			if err != nil {
				errorsFound <- err
				return
			}
			versions <- created.Version()
		}(index)
	}
	waitGroup.Wait()
	close(versions)
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("concurrent memory write: %v", err)
	}
	seen := make(map[int]bool, writers)
	for version := range versions {
		if version < 1 || version > writers || seen[version] {
			t.Errorf("allocated version %d is out of range or duplicated", version)
		}
		seen[version] = true
	}
	if len(seen) != writers {
		t.Fatalf("allocated %d unique versions, want %d", len(seen), writers)
	}

	duplicate, err := memory.New("memory-00", "memory-project", memory.CategoryKnownProblem, 1,
		"duplicate identifier", "brain", "brain-1", nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateNext(ctx, duplicate); err == nil {
		t.Fatal("CreateNext() accepted a duplicate summary identifier")
	}
	next, err := memory.New("memory-next", "memory-project", memory.CategoryKnownProblem, 1,
		"next summary", "brain", "brain-1", nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.CreateNext(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	if created.Version() != writers+1 {
		t.Fatalf("version after rolled back insert = %d, want %d", created.Version(), writers+1)
	}
}
