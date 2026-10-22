package task_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/credential"
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
	otherProject, err := projects.Create(ctx, project.CreateInput{Name: "Other project"})
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
	worker, err := agents.Create(ctx, agent.CreateInput{Name: "Worker", Role: "worker", Runtime: agent.RuntimeLocal, Capabilities: []string{"task.transition", "task.complete"}})
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

	local := task.Actor{Type: task.ActorAdministrator, ID: "local-administrator", Capabilities: []string{"task.create", "task.assign", "task.transition", "task.review", "task.complete"}, ProjectIDs: []string{p.ID(), otherProject.ID()}}
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
	reloadedWorker, err := agents.Get(ctx, worker.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(reloadedWorker.ProjectIDs()) != 0 {
		t.Fatalf("reloaded worker project scope = %v, want empty before project membership is implemented", reloadedWorker.ProjectIDs())
	}
	value, err = service.Transition(ctx, value.ID(), task.StatusReady, "prepare assigned task", local)
	if err != nil {
		t.Fatal(err)
	}
	value, err = service.Transition(ctx, value.ID(), task.StatusWorking, "work on assigned task", workerActor)
	if err != nil {
		t.Fatalf("assigned worker transition in task project: %v", err)
	}
	otherTask, err := service.Create(ctx, otherProject.ID(), task.CreateInput{Title: "Unassigned other project task", Reason: "test scope boundary"}, local)
	if err != nil {
		t.Fatal(err)
	}
	otherTask, err = service.Transition(ctx, otherTask.ID(), task.StatusReady, "prepare other project task", local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Transition(ctx, otherTask.ID(), task.StatusWorking, "attempt other project task", workerActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("worker transition on unassigned task in another project = %v, want forbidden", err)
	}
	otherTask, err = taskRepository.Get(ctx, otherTask.ID())
	if err != nil {
		t.Fatal(err)
	}
	if otherTask.Status() != task.StatusReady {
		t.Fatalf("other project task status = %s after denied transition, want READY", otherTask.Status())
	}
	if _, err := service.Transition(ctx, value.ID(), task.StatusDone, "skip review", workerActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("worker completion error = %v, want forbidden", err)
	}
	for _, status := range []task.Status{task.StatusReview, task.StatusHumanReview, task.StatusDone} {
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
	if len(events) != 8 {
		t.Fatalf("audit event count = %d, want create + assignment + 5 transitions + review approval", len(events))
	}
	noReview := false
	noReviewProject, err := projects.Create(ctx, project.CreateInput{Name: "No review", HumanReviewRequired: &noReview})
	if err != nil {
		t.Fatal(err)
	}
	local.ProjectIDs = append(local.ProjectIDs, noReviewProject.ID())
	completeTask, err := service.Create(ctx, noReviewProject.ID(), task.CreateInput{Title: "Complete without review", Reason: "completion capability test"}, local)
	if err != nil {
		t.Fatal(err)
	}
	completeTask, err = service.Assign(ctx, completeTask.ID(), worker.ID(), "assign completion task", local)
	if err != nil {
		t.Fatal(err)
	}
	completeTask, err = service.Transition(ctx, completeTask.ID(), task.StatusReady, "make available", local)
	if err != nil {
		t.Fatal(err)
	}
	completeTask, err = service.Transition(ctx, completeTask.ID(), task.StatusWorking, "begin work", workerActor)
	if err != nil {
		t.Fatal(err)
	}
	completeTask, err = service.Transition(ctx, completeTask.ID(), task.StatusReview, "submit for completion", workerActor)
	if err != nil {
		t.Fatal(err)
	}
	completeTask, err = service.Transition(ctx, completeTask.ID(), task.StatusDone, "work complete", workerActor)
	if err != nil {
		t.Fatalf("assigned worker task.complete without Human Review: %v", err)
	}
	if completeTask.Status() != task.StatusDone {
		t.Fatalf("task status = %s, want DONE", completeTask.Status())
	}
	approved := false
	for _, event := range events {
		wantActor := local.ID
		if event.ToStatus == string(task.StatusWorking) {
			wantActor = worker.ID()
		}
		if event.ActorID != wantActor || event.Reason == "" {
			t.Fatalf("audit event missing actor/reason: %+v", event)
		}
		if event.EventType == "task.human_review.approved" {
			approved = event.ActorID == local.ID && event.ToStatus == string(task.StatusDone) && event.Reason == "advance workflow" && !event.CreatedAt.IsZero()
		}
	}
	if !approved {
		t.Fatal("human review approval was not fully recorded")
	}
	rejectedTask, err := service.Create(ctx, p.ID(), task.CreateInput{Title: "Rejected task", Reason: "test rejection"}, local)
	if err != nil {
		t.Fatal(err)
	}
	rejectedTask, err = service.Assign(ctx, rejectedTask.ID(), worker.ID(), "assign for review", local)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []task.Status{task.StatusReady, task.StatusWorking, task.StatusReview, task.StatusHumanReview} {
		rejectedTask, err = service.Transition(ctx, rejectedTask.ID(), status, "submit for approval", local)
		if err != nil {
			t.Fatalf("Transition(%s): %v", status, err)
		}
	}
	if _, err := service.Transition(ctx, rejectedTask.ID(), task.StatusDone, "worker bypass", workerActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("worker completion while review is pending = %v, want forbidden", err)
	}
	if _, err := service.Transition(ctx, rejectedTask.ID(), task.StatusWorking, "worker rejection attempt", workerActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("worker human review rejection = %v, want forbidden", err)
	}
	if _, err := service.Transition(ctx, rejectedTask.ID(), task.StatusCancelled, "worker cancellation attempt", workerActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("worker human review cancellation = %v, want forbidden", err)
	}
	var denialReason string
	if err := database.QueryRowContext(ctx, `SELECT reason FROM authorization_denials WHERE resource='task:' || ? ORDER BY created_at DESC LIMIT 1`, rejectedTask.ID()).Scan(&denialReason); err != nil {
		t.Fatal(err)
	}
	if denialReason != "human_review_action_requires_administrator" {
		t.Fatalf("worker human review denial reason = %q", denialReason)
	}
	preDecisionEvents, err := service.ListAuditEvents(ctx, rejectedTask.ID())
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range preDecisionEvents {
		if event.EventType == "task.human_review.rejected" {
			t.Fatalf("AI action was recorded as a human rejection: %+v", event)
		}
	}
	if _, err := service.Transition(ctx, rejectedTask.ID(), task.StatusWorking, "needs changes", local); err != nil {
		t.Fatalf("reject human review: %v", err)
	}
	rejectedEvents, err := service.ListAuditEvents(ctx, rejectedTask.ID())
	if err != nil {
		t.Fatal(err)
	}
	rejected := false
	for _, event := range rejectedEvents {
		if event.EventType == "task.human_review.rejected" {
			rejected = event.ActorID == local.ID && event.ToStatus == string(task.StatusWorking) && event.Reason == "needs changes" && !event.CreatedAt.IsZero()
		}
	}
	if !rejected {
		t.Fatal("human review rejection was not fully recorded")
	}
}

type synchronizedTaskRepository struct {
	task.Repository
	mu      sync.Mutex
	reads   int
	ready   chan struct{}
	release chan struct{}
}

func (r *synchronizedTaskRepository) Get(ctx context.Context, id string) (task.Task, error) {
	value, err := r.Repository.Get(ctx, id)
	if err != nil {
		return task.Task{}, err
	}
	r.mu.Lock()
	r.reads++
	if r.reads == 2 {
		close(r.ready)
	}
	r.mu.Unlock()
	select {
	case <-r.release:
		return value, nil
	case <-ctx.Done():
		return task.Task{}, ctx.Err()
	}
}

func TestConcurrentHumanReviewDecisionsRejectStaleMutation(t *testing.T) {
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
	p, err := projects.Create(ctx, project.CreateInput{Name: "Concurrent review", HumanReviewRequired: &reviewRequired})
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
	brainRepository, err := storage.NewBrainRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	baseRepository, err := storage.NewTaskRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	baseService, err := task.NewService(baseRepository, projects, agents, brainRepository)
	if err != nil {
		t.Fatal(err)
	}
	admin := task.Actor{Type: task.ActorAdministrator, ID: "admin", Capabilities: []string{"task.create", "task.transition", "task.review", "task.complete"}, ProjectIDs: []string{p.ID()}}
	value, err := baseService.Create(ctx, p.ID(), task.CreateInput{Title: "Review decision", Reason: "test concurrency"}, admin)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []task.Status{task.StatusReady, task.StatusWorking, task.StatusReview, task.StatusHumanReview} {
		value, err = baseService.Transition(ctx, value.ID(), status, "advance to review", admin)
		if err != nil {
			t.Fatalf("transition to %s: %v", status, err)
		}
	}
	gated := &synchronizedTaskRepository{Repository: baseRepository, ready: make(chan struct{}), release: make(chan struct{})}
	concurrentService, err := task.NewService(gated, projects, agents, brainRepository)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	go func() {
		_, err := concurrentService.Transition(ctx, value.ID(), task.StatusDone, "approve review", admin)
		results <- err
	}()
	go func() {
		_, err := concurrentService.Transition(ctx, value.ID(), task.StatusWorking, "reject review", admin)
		results <- err
	}()
	<-gated.ready
	close(gated.release)
	first, second := <-results, <-results
	successes, conflicts := 0, 0
	for _, result := range []error{first, second} {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, task.ErrConflict):
			conflicts++
		default:
			t.Fatalf("concurrent review decision error = %v, want success or ErrConflict", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent decisions: successes=%d conflicts=%d, want one each", successes, conflicts)
	}
	current, err := baseRepository.Get(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	if current.Status() != task.StatusDone && current.Status() != task.StatusWorking {
		t.Fatalf("final status = %s, want DONE or WORKING", current.Status())
	}
	events, err := baseRepository.ListAuditEvents(ctx, value.ID())
	if err != nil {
		t.Fatal(err)
	}
	decisions := 0
	for _, event := range events {
		if event.EventType == "task.human_review.approved" || event.EventType == "task.human_review.rejected" {
			decisions++
		}
	}
	if decisions != 1 {
		t.Fatalf("Human Review decision audit count = %d, want 1", decisions)
	}
}

var _ task.BrainReader = (*storage.BrainRepository)(nil)

func TestBrainTaskActionsFollowAutonomyAndHumanReviewPolicy(t *testing.T) {
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "autonomy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	projectRepository, _ := storage.NewProjectRepository(database)
	projects, _ := project.NewManager(projectRepository)
	humanReview := false
	p, err := projects.Create(ctx, project.CreateInput{Name: "Autonomy", HumanReviewRequired: &humanReview})
	if err != nil {
		t.Fatal(err)
	}
	credentialRepository, _ := storage.NewCredentialRepository(database)
	credentials, err := credential.NewManager(credentialRepository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	secret, err := credentials.Create(ctx, "test", "openai-compatible", "provider-secret")
	if err != nil {
		t.Fatal(err)
	}
	agentRepository, _ := storage.NewAgentRepository(database)
	agents, _ := agent.NewManager(agentRepository)
	configuredAgent, err := agents.Create(ctx, agent.CreateInput{
		Name: "Brain", Role: "coordinator", Runtime: agent.RuntimeAPI, Provider: "openai-compatible", Model: "test-model", CredentialID: secret.ID,
		Capabilities: []string{"brain.execute", "task.create", "task.assign", "task.review", "task.complete", "task.transition"},
	})
	if err != nil {
		t.Fatal(err)
	}
	brainRepository, _ := storage.NewBrainRepository(database)
	runRepository, _ := storage.NewRunRepository(database)
	brainService, err := brain.NewService(brainRepository, projects, agents, runRepository, credentials)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := brainService.Create(ctx, p.ID(), brain.CreateInput{AgentID: configuredAgent.ID(), Autonomy: brain.AutonomyManual}); err != nil {
		t.Fatal(err)
	}
	taskRepository, _ := storage.NewTaskRepository(database)
	tasks, err := task.NewService(taskRepository, projects, agents, brainRepository)
	if err != nil {
		t.Fatal(err)
	}
	brainActor := task.Actor{Type: task.ActorBrain, ID: configuredAgent.ID()}
	if _, err := tasks.Create(ctx, p.ID(), task.CreateInput{Title: "Manual task", Reason: "manual mode"}, brainActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("manual mode task creation error = %v, want forbidden", err)
	}
	if _, err := brainService.Replace(ctx, p.ID(), brain.CreateInput{AgentID: configuredAgent.ID(), Autonomy: brain.AutonomySemiAuto}, project.LocalAdministratorID, "enable semi-auto"); err != nil {
		t.Fatal(err)
	}
	value, err := tasks.Create(ctx, p.ID(), task.CreateInput{Title: "Semi-auto task", Reason: "semi-auto mode"}, brainActor)
	if err != nil {
		t.Fatalf("semi-auto task creation: %v", err)
	}
	for _, status := range []task.Status{task.StatusReady, task.StatusWorking, task.StatusReview} {
		value, err = tasks.Transition(ctx, value.ID(), status, "advance task", brainActor)
		if err != nil {
			t.Fatalf("semi-auto transition to %s: %v", status, err)
		}
	}
	if _, err := tasks.Transition(ctx, value.ID(), task.StatusDone, "semi-auto completion", brainActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("semi-auto completion error = %v, want forbidden", err)
	}
	var denialReason string
	if err := database.QueryRowContext(ctx, `SELECT reason FROM authorization_denials WHERE resource='task:' || ? ORDER BY created_at DESC LIMIT 1`, value.ID()).Scan(&denialReason); err != nil {
		t.Fatal(err)
	}
	if denialReason != "human_approval_required" {
		t.Fatalf("semi-auto completion denial reason = %q", denialReason)
	}
	if _, err := brainService.Replace(ctx, p.ID(), brain.CreateInput{AgentID: configuredAgent.ID(), Autonomy: brain.AutonomyAutonomous}, project.LocalAdministratorID, "enable autonomous mode"); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Transition(ctx, value.ID(), task.StatusDone, "autonomous completion", brainActor); err != nil {
		t.Fatalf("autonomous completion without human-review policy: %v", err)
	}
	reviewRequired := true
	reviewProject, err := projects.Create(ctx, project.CreateInput{Name: "Human review", HumanReviewRequired: &reviewRequired})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := brainService.Create(ctx, reviewProject.ID(), brain.CreateInput{AgentID: configuredAgent.ID(), Autonomy: brain.AutonomyAutonomous}); err != nil {
		t.Fatal(err)
	}
	reviewTask, err := tasks.Create(ctx, reviewProject.ID(), task.CreateInput{Title: "Protected task", Reason: "verify review gate"}, brainActor)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []task.Status{task.StatusReady, task.StatusWorking, task.StatusReview, task.StatusHumanReview} {
		reviewTask, err = tasks.Transition(ctx, reviewTask.ID(), status, "advance to human review", brainActor)
		if err != nil {
			t.Fatalf("autonomous transition to %s: %v", status, err)
		}
	}
	if _, err := tasks.Transition(ctx, reviewTask.ID(), task.StatusDone, "attempt review bypass", brainActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("Brain completion with pending human review = %v, want forbidden", err)
	}
	if _, err := tasks.Transition(ctx, reviewTask.ID(), task.StatusWorking, "attempt human rejection", brainActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("Brain human review rejection = %v, want forbidden", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT reason FROM authorization_denials WHERE resource='task:' || ? ORDER BY created_at DESC LIMIT 1`, reviewTask.ID()).Scan(&denialReason); err != nil {
		t.Fatal(err)
	}
	if denialReason != "human_review_action_requires_administrator" {
		t.Fatalf("Brain human review denial reason = %q", denialReason)
	}
	if _, err := tasks.Transition(ctx, reviewTask.ID(), task.StatusCancelled, "attempt human review cancellation", brainActor); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("Brain human review cancellation = %v, want forbidden", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT reason FROM authorization_denials WHERE resource='task:' || ? ORDER BY created_at DESC LIMIT 1`, reviewTask.ID()).Scan(&denialReason); err != nil {
		t.Fatal(err)
	}
	if denialReason != "human_review_action_requires_administrator" {
		t.Fatalf("Brain human review cancellation denial reason = %q", denialReason)
	}
	reviewTask, err = taskRepository.Get(ctx, reviewTask.ID())
	if err != nil {
		t.Fatal(err)
	}
	if reviewTask.Status() != task.StatusHumanReview {
		t.Fatalf("Brain rejection attempt left task in %s, want HUMAN_REVIEW", reviewTask.Status())
	}
	reviewEvents, err := tasks.ListAuditEvents(ctx, reviewTask.ID())
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range reviewEvents {
		if event.EventType == "task.human_review.rejected" {
			t.Fatalf("Brain action was recorded as a human rejection: %+v", event)
		}
	}
}
