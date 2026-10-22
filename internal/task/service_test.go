package task_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/project"
	"boAIrd/internal/storage"
	"boAIrd/internal/task"
)

func TestServiceEnforcesWorkflowAndRecordsAudit(t *testing.T) {
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	projectRepository, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	reviewRequired := true
	p, err := projects.Create(ctx, project.CreateInput{Name: "Workflow", HumanReviewRequired: &reviewRequired})
	if err != nil {
		t.Fatal(err)
	}

	agentRepository, err := storage.NewAgentRepository(database)
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

	brainRepository, err := storage.NewBrainRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	taskRepository, err := storage.NewTaskRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	service, err := task.NewService(taskRepository, projects, agents, brainRepository)
	if err != nil {
		t.Fatal(err)
	}

	local := task.Actor{Type: task.ActorAdministrator, ID: "local-administrator", Capabilities: []string{"task.create", "task.assign", "task.transition", "task.review", "task.complete"}, ProjectIDs: []string{p.ID()}}
	workerActor := task.Actor{Type: task.ActorAgent, ID: worker.ID()}
	if _, err := service.Create(ctx, p.ID(), task.CreateInput{Title: "Unauthorized", Reason: "test"}, workerActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("worker Create() error = %v, want forbidden", err)
	}
	if _, err := service.Create(ctx, p.ID(), task.CreateInput{Title: "Legacy actor", Reason: "test"}, task.Actor{Type: task.ActorLocalAPI, ID: "local-api"}); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("legacy local actor Create() error = %v, want forbidden", err)
	}
	var denials int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM authorization_denials WHERE required_capability='task.create'").Scan(&denials); err != nil {
		t.Fatal(err)
	}
	if denials != 2 {
		t.Fatalf("authorization denial audit count = %d, want 2", denials)
	}
	value, err := service.Create(ctx, p.ID(), task.CreateInput{Title: "Implement workflow", Reason: "accepted request"}, local)
	if err != nil {
		t.Fatal(err)
	}
	if value.Status() != task.StatusBacklog || !value.HumanReview() {
		t.Fatalf("new task state = %s, human review = %v", value.Status(), value.HumanReview())
	}
	value, err = service.Assign(ctx, value.ID(), worker.ID(), "assign to worker", local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Transition(ctx, value.ID(), task.StatusDone, "skip review", workerActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("worker completion error = %v, want forbidden", err)
	}
	for _, status := range []task.Status{task.StatusReady, task.StatusWorking, task.StatusReview, task.StatusHumanReview, task.StatusDone} {
		value, err = service.Transition(ctx, value.ID(), status, "advance workflow", local)
		if err != nil {
			t.Fatalf("Transition(%s): %v", status, err)
		}
	}
	if _, err := service.Transition(ctx, value.ID(), task.StatusWorking, "reopen", local); !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("invalid terminal transition error = %v, want invalid transition", err)
	}
	events, err := service.ListAuditEvents(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 7 {
		t.Fatalf("audit event count = %d, want create + assignment + 5 transitions", len(events))
	}
	for _, event := range events {
		if event.ActorID != local.ID || event.Reason == "" {
			t.Fatalf("audit event missing actor/reason: %+v", event)
		}
	}
}

var _ task.BrainReader = (*storage.BrainRepository)(nil)
