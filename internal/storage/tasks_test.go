package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/project"
	"boAIrd/internal/task"
)

func TestTaskUpdateRejectsStaleVersionWithoutAuditMutation(t *testing.T) {
	ctx := context.Background()
	database, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "tasks.db"))
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
	p, err := projects.Create(ctx, project.CreateInput{Name: "Concurrency"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := NewTaskRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	actor := task.Actor{Type: task.ActorAdministrator, ID: "admin"}
	value, err := task.NewWithActor("task-1", p.ID(), "Concurrent task", "", actor)
	if err != nil {
		t.Fatal(err)
	}
	value, err = value.RecordMutation(actor, "create task")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, value); err != nil {
		t.Fatal(err)
	}
	winner, err := repository.Get(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	stale, err := repository.Get(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	winner, err = winner.Transition(task.StatusReady)
	if err != nil {
		t.Fatal(err)
	}
	winner, err = winner.RecordMutation(actor, "publish task")
	if err != nil {
		t.Fatal(err)
	}
	winner, err = winner.AdvanceVersion()
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Update(ctx, winner); err != nil {
		t.Fatalf("winner update: %v", err)
	}
	stale, err = stale.Transition(task.StatusReady)
	if err != nil {
		t.Fatal(err)
	}
	stale, err = stale.RecordMutation(actor, "stale publish")
	if err != nil {
		t.Fatal(err)
	}
	stale, err = stale.AdvanceVersion()
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Update(ctx, stale); !errors.Is(err, task.ErrConflict) {
		t.Fatalf("stale Update() error = %v, want ErrConflict", err)
	}
	current, err := repository.Get(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	if current.Status() != task.StatusReady || current.Version() != 2 || current.LastReason() != "publish task" {
		t.Fatalf("persisted task = status %s, version %d, reason %q; want READY, 2, and winner reason", current.Status(), current.Version(), current.LastReason())
	}
	events, err := repository.ListAuditEvents(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	var statusChanges int
	for _, event := range events {
		if event.EventType == "task.status.changed" {
			statusChanges++
			if event.Reason != "publish task" {
				t.Fatalf("status audit reason = %q, want winner reason", event.Reason)
			}
		}
	}
	if statusChanges != 1 {
		t.Fatalf("status change audit count = %d, want 1", statusChanges)
	}
}

func TestTaskUpdateDistinguishesMissingTaskFromConflict(t *testing.T) {
	ctx := context.Background()
	database, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	repository, err := NewTaskRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	value, err := task.New("missing", "project", "Missing", "", "admin")
	if err != nil {
		t.Fatal(err)
	}
	value, err = value.AdvanceVersion()
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Update(ctx, value); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestStaleTransitionCannotOverwriteConcurrentAssignment(t *testing.T) {
	ctx := context.Background()
	database, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "tasks.db"))
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
	p, err := projects.Create(ctx, project.CreateInput{Name: "Assignment concurrency"})
	if err != nil {
		t.Fatal(err)
	}
	agentRepository, err := NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	agents, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := agents.Create(ctx, agent.CreateInput{Name: "Worker", Role: "worker", Runtime: agent.RuntimeLocal})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := NewTaskRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	actor := task.Actor{Type: task.ActorAdministrator, ID: "admin"}
	value, err := task.NewWithActor("task-1", p.ID(), "Concurrent task", "", actor)
	if err != nil {
		t.Fatal(err)
	}
	value, err = value.RecordMutation(actor, "create task")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, value); err != nil {
		t.Fatal(err)
	}
	winner, err := repository.Get(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	stale, err := repository.Get(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	winner, err = winner.Assign(worker.ID())
	if err != nil {
		t.Fatal(err)
	}
	winner, err = winner.RecordMutation(actor, "assign worker")
	if err != nil {
		t.Fatal(err)
	}
	winner, err = winner.AdvanceVersion()
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Update(ctx, winner); err != nil {
		t.Fatalf("assignment update: %v", err)
	}
	stale, err = stale.Transition(task.StatusReady)
	if err != nil {
		t.Fatal(err)
	}
	stale, err = stale.RecordMutation(actor, "publish stale task")
	if err != nil {
		t.Fatal(err)
	}
	stale, err = stale.AdvanceVersion()
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Update(ctx, stale); !errors.Is(err, task.ErrConflict) {
		t.Fatalf("stale transition update error = %v, want ErrConflict", err)
	}
	current, err := repository.Get(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	if current.Status() != task.StatusBacklog || current.AssignedTo() != worker.ID() || current.Version() != 2 {
		t.Fatalf("task after stale transition = status %s, assignee %q, version %d; want BACKLOG, assigned worker, version 2", current.Status(), current.AssignedTo(), current.Version())
	}
}
