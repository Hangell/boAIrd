package orchestrator_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/audit"
	"boAIrd/internal/brain"
	"boAIrd/internal/conversation"
	"boAIrd/internal/credential"
	"boAIrd/internal/memory"
	"boAIrd/internal/orchestrator"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/runtime"
	"boAIrd/internal/storage"
	"boAIrd/internal/task"
)

type fakeExecutor struct {
	mu      sync.Mutex
	calls   []runtime.Request
	outputs []string
	errs    []error
}

type fakeMemorySummarizer struct{}

func (fakeMemorySummarizer) Summarize(_ context.Context, _, objective string, _ int) (brain.Execution, error) {
	if !strings.Contains(objective, "delivered work") {
		return brain.Execution{}, errors.New("summary input is missing completed Run output")
	}
	return brain.Execution{Content: "The assigned change was implemented and delivered."}, nil
}

func (f *fakeExecutor) Execute(_ context.Context, request runtime.Request) (runtime.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, request)
	index := len(f.calls) - 1
	if index < len(f.errs) && f.errs[index] != nil {
		return runtime.Result{}, f.errs[index]
	}
	if index < len(f.outputs) {
		return runtime.Result{Output: f.outputs[index]}, nil
	}
	return runtime.Result{Output: "delivered work"}, nil
}

func (f *fakeExecutor) Calls() []runtime.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]runtime.Request(nil), f.calls...)
}

type fakeEscalations struct {
	mu     sync.Mutex
	values []audit.Escalation
}

func (f *fakeEscalations) RecordEscalation(_ context.Context, escalation audit.Escalation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values = append(f.values, escalation)
	return nil
}

func (f *fakeEscalations) Values() []audit.Escalation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]audit.Escalation(nil), f.values...)
}

type fixture struct {
	database        *sql.DB
	projects        *project.Manager
	agents          *agent.Manager
	tasks           *task.Service
	runs            *storage.RunRepository
	brainRepository orchestrator.BrainReader
	conversations   *conversation.Service
	brainService    *brain.Service
	memory          *memory.Service
	project         project.Project
	brain           brain.Brain
	worker          agent.Agent
	brainAgent      agent.Agent
	executor        *fakeExecutor
	escalations     *fakeEscalations
	service         *orchestrator.Service
}

type fixtureOptions struct {
	humanReview   bool
	maxAttempts   int
	brainCaps     []string
	workerRuntime agent.Runtime
}

func newFixture(t *testing.T, options fixtureOptions) *fixture {
	t.Helper()
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "orchestrator.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	projectRepository, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projectManager, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	credentialRepository, err := storage.NewCredentialRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	credentialManager, err := credential.NewManager(credentialRepository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := credentialManager.Create(ctx, "worker", "openai-compatible", "provider-secret")
	if err != nil {
		t.Fatal(err)
	}
	value, err := projectManager.Create(ctx, project.CreateInput{Name: "Orchestration", HumanReviewRequired: &options.humanReview})
	if err != nil {
		t.Fatal(err)
	}
	agentRepository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	agentManager, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	brainCaps := options.brainCaps
	if brainCaps == nil {
		brainCaps = []string{"brain.execute", "task.assign", "task.review", "task.complete"}
	}
	brainAgent, err := agentManager.Create(ctx, agent.CreateInput{
		Name: "Project Brain", Role: "coordinator", Runtime: agent.RuntimeAPI, Provider: "openai-compatible",
		Model: "test-model", CredentialID: metadata.ID, Capabilities: brainCaps,
	})
	if err != nil {
		t.Fatal(err)
	}
	workerRuntime := options.workerRuntime
	if workerRuntime == "" {
		workerRuntime = agent.RuntimeAPI
	}
	workerInput := agent.CreateInput{Name: "Worker", Role: "backend", Runtime: workerRuntime, Capabilities: []string{"task.transition"}}
	if workerRuntime == agent.RuntimeAPI {
		workerInput.Provider = "openai-compatible"
		workerInput.Model = "test-model"
		workerInput.CredentialID = metadata.ID
	}
	worker, err := agentManager.Create(ctx, workerInput)
	if err != nil {
		t.Fatal(err)
	}
	brainRepository, err := storage.NewBrainRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	runRepository, err := storage.NewRunRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	brainService, err := brain.NewService(brainRepository, projectManager, agentManager, runRepository, credentialManager)
	if err != nil {
		t.Fatal(err)
	}
	memoryRepository, err := storage.NewMemoryRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	memoryService, err := memory.NewService(memoryRepository, projectManager, memory.Config{})
	if err != nil {
		t.Fatal(err)
	}
	configuredBrain, err := brainService.Create(ctx, value.ID(), brain.CreateInput{AgentID: brainAgent.ID(), Autonomy: brain.AutonomyAutonomous})
	if err != nil {
		t.Fatal(err)
	}
	taskRepository, err := storage.NewTaskRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	taskService, err := task.NewService(taskRepository, projectManager, agentManager, brainRepository)
	if err != nil {
		t.Fatal(err)
	}
	conversationRepository, err := storage.NewConversationRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	conversationService, err := conversation.NewService(conversationRepository, taskService, projectManager, agentManager, brainRepository)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{}
	escalations := &fakeEscalations{}
	service, err := orchestrator.New(orchestrator.Dependencies{
		Tasks: taskService, Runs: runRepository, Agents: agentManager, Projects: projectManager,
		Brains: brainRepository, Executor: executor, Conversations: conversationService, ContextReader: conversationService, Escalations: escalations,
		Memory: memoryService, Summarizer: fakeMemorySummarizer{},
		Config: orchestrator.Config{MaxAttempts: options.maxAttempts},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{
		database: database, projects: projectManager, agents: agentManager, tasks: taskService, runs: runRepository,
		brainRepository: brainRepository, conversations: conversationService, brainService: brainService, memory: memoryService, project: value, brain: configuredBrain,
		worker: worker, brainAgent: brainAgent, executor: executor, escalations: escalations, service: service,
	}
}

func (f *fixture) adminActor() task.Actor {
	return task.Actor{
		Type: task.ActorAdministrator, ID: project.LocalAdministratorID,
		Capabilities: []string{"task.create", "task.assign", "task.transition", "task.review", "task.complete"},
		ProjectIDs:   []string{f.project.ID()},
	}
}

func (f *fixture) readyTask(t *testing.T, ctx context.Context) task.Task {
	t.Helper()
	created, err := f.tasks.Create(ctx, f.project.ID(), task.CreateInput{
		Title: "Implement the assigned change", Description: "Deliver the requested work.", Reason: "plan work",
	}, f.adminActor())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.tasks.Assign(ctx, created.ID(), f.worker.ID(), "assign worker", f.adminActor()); err != nil {
		t.Fatal(err)
	}
	ready, err := f.tasks.Transition(ctx, created.ID(), task.StatusReady, "ready for orchestration", f.adminActor())
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func TestDispatchExecutesTaskAndReviewCompletes(t *testing.T) {
	f := newFixture(t, fixtureOptions{maxAttempts: 3})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)
	if _, err := f.memory.Record(ctx, f.project.ID(), memory.RecordInput{
		Category: memory.CategoryKnownProblem, Content: "assigned change persist safely", CreatedByType: "brain", CreatedBy: f.brain.AgentID(),
	}); err != nil {
		t.Fatal(err)
	}

	report, err := f.service.Dispatch(ctx, f.project.ID())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != orchestrator.OutcomeSucceeded {
		t.Fatalf("dispatch outcomes = %+v", report.Outcomes)
	}
	selection := report.Outcomes[0].ContextSelection
	if selection == nil || selection.ProjectID != f.project.ID() || selection.TaskID != ready.ID() {
		t.Fatalf("context selection = %+v", selection)
	}
	if len(selection.Selected) < 2 {
		t.Fatalf("context selection omitted project/task inputs: %+v", selection.Selected)
	}
	if !strings.Contains(f.executor.Calls()[0].Input, "assigned change persist safely") {
		t.Fatalf("executor input missing selected project memory: %q", f.executor.Calls()[0].Input)
	}
	calls := f.executor.Calls()
	if len(calls) != 1 || calls[0].TaskID != ready.ID() || calls[0].AgentID != f.worker.ID() {
		t.Fatalf("executor calls = %+v", calls)
	}
	if !strings.Contains(calls[0].Input, ready.Title()) {
		t.Fatalf("executor input missing task title: %q", calls[0].Input)
	}
	runs, err := f.runs.List(ctx, f.project.ID(), ready.ID(), "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status() != run.StatusSuccess || runs[0].Output() != "delivered work" {
		t.Fatalf("runs = %+v", runs)
	}
	if runs[0].AgentID() != f.worker.ID() || runs[0].TaskID() != ready.ID() {
		t.Fatalf("run is not linked to the task and worker: %+v", runs[0])
	}
	reviewing, err := f.tasks.Get(ctx, ready.ID())
	if err != nil {
		t.Fatal(err)
	}
	if reviewing.Status() != task.StatusReview {
		t.Fatalf("task status after execution = %s", reviewing.Status())
	}
	assertTransitions(t, f.database, ready.ID(), [][2]string{{"READY", "WORKING"}, {"WORKING", "REVIEW"}})

	completed, err := f.service.Review(ctx, ready.ID(), orchestrator.DecisionApprove, "looks good")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if completed.Status() != task.StatusDone {
		t.Fatalf("task status after approval = %s", completed.Status())
	}
	memoryValues, err := f.memory.List(ctx, f.project.ID())
	if err != nil {
		t.Fatal(err)
	}
	foundTaskSummary := false
	for _, value := range memoryValues {
		if value.Category() != memory.CategoryTaskHistory {
			continue
		}
		foundTaskSummary = true
		if !strings.Contains(value.Content(), "implemented and delivered") || len(value.SourceIDs()) < 2 || value.CreatedByType() != "brain" {
			t.Fatalf("unexpected generated task memory: %+v", value)
		}
	}
	if !foundTaskSummary {
		t.Fatal("Brain approval did not persist a task history summary")
	}
}

func TestReviewReworkReexecutesWithInstructions(t *testing.T) {
	f := newFixture(t, fixtureOptions{maxAttempts: 3})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)
	if _, err := f.service.Dispatch(ctx, f.project.ID()); err != nil {
		t.Fatal(err)
	}

	reworked, err := f.service.Review(ctx, ready.ID(), orchestrator.DecisionRework, "add regression tests")
	if err != nil {
		t.Fatalf("rework: %v", err)
	}
	if reworked.Status() != task.StatusReview {
		t.Fatalf("task status after rework = %s", reworked.Status())
	}
	calls := f.executor.Calls()
	if len(calls) != 2 {
		t.Fatalf("executor calls = %d, want 2", len(calls))
	}
	if !strings.Contains(calls[1].Input, "add regression tests") {
		t.Fatalf("rework input missing instructions: %q", calls[1].Input)
	}
	if !strings.Contains(calls[1].Input, "delivered work") {
		t.Fatalf("rework input missing prior Run output: %q", calls[1].Input)
	}
	runs, err := f.runs.List(ctx, f.project.ID(), ready.ID(), "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(runs))
	}
}

func TestFailureRetriesThenEscalates(t *testing.T) {
	f := newFixture(t, fixtureOptions{maxAttempts: 2})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)
	f.executor.errs = []error{errors.New("worker exploded"), errors.New("worker exploded again")}

	first, err := f.service.Dispatch(ctx, f.project.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Outcomes) != 1 || first.Outcomes[0].Status != orchestrator.OutcomeRetryScheduled {
		t.Fatalf("first dispatch outcomes = %+v", first.Outcomes)
	}
	retrying, err := f.tasks.Get(ctx, ready.ID())
	if err != nil {
		t.Fatal(err)
	}
	if retrying.Status() != task.StatusReady {
		t.Fatalf("task status after first failure = %s", retrying.Status())
	}
	if len(f.escalations.Values()) != 0 {
		t.Fatalf("unexpected escalation after retryable failure")
	}
	_, messages, _, err := f.conversations.GetPage(ctx, ready.ID(), project.LocalAdministratorID, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) == 0 || messages[len(messages)-1].SenderType() != conversation.SenderBrain || !strings.Contains(messages[len(messages)-1].Content(), "Retry instructions") {
		t.Fatalf("Brain retry instructions were not recorded")
	}

	second, err := f.service.Dispatch(ctx, f.project.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Outcomes) != 1 || second.Outcomes[0].Status != orchestrator.OutcomeEscalated {
		t.Fatalf("second dispatch outcomes = %+v", second.Outcomes)
	}
	blocked, err := f.tasks.Get(ctx, ready.ID())
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status() != task.StatusBlocked {
		t.Fatalf("task status after exhausted retries = %s", blocked.Status())
	}
	runs, err := f.runs.List(ctx, f.project.ID(), ready.ID(), "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(runs))
	}
	for _, value := range runs {
		if value.Status() != run.StatusFailed {
			t.Fatalf("run status = %s, want FAILED", value.Status())
		}
	}
	escalations := f.escalations.Values()
	if len(escalations) != 1 || escalations[0].TaskID != ready.ID() || escalations[0].Attempts != 2 {
		t.Fatalf("escalations = %+v", escalations)
	}

	third, err := f.service.Dispatch(ctx, f.project.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Outcomes) != 0 {
		t.Fatalf("blocked task was dispatched again: %+v", third.Outcomes)
	}
}

func TestHumanReviewIsPreserved(t *testing.T) {
	f := newFixture(t, fixtureOptions{humanReview: true, maxAttempts: 3})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)
	if _, err := f.service.Dispatch(ctx, f.project.ID()); err != nil {
		t.Fatal(err)
	}

	reviewed, err := f.service.Review(ctx, ready.ID(), orchestrator.DecisionApprove, "approved by Brain")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if reviewed.Status() != task.StatusHumanReview {
		t.Fatalf("task status = %s, want HUMAN_REVIEW", reviewed.Status())
	}
	if _, err := f.service.Review(ctx, ready.ID(), orchestrator.DecisionApprove, "again"); !errors.Is(err, orchestrator.ErrTaskNotInReview) {
		t.Fatalf("second review error = %v, want ErrTaskNotInReview", err)
	}
	memoryValues, err := f.memory.List(ctx, f.project.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(memoryValues) != 0 {
		t.Fatalf("Brain persisted memory before human approval: %+v", memoryValues)
	}

	completed, err := f.tasks.Transition(ctx, ready.ID(), task.StatusDone, "administrator approved", f.adminActor())
	if err != nil {
		t.Fatalf("administrator completion: %v", err)
	}
	if completed.Status() != task.StatusDone {
		t.Fatalf("task status = %s, want DONE", completed.Status())
	}
	if err := f.service.RecordHumanReviewCompletion(ctx, completed); err != nil {
		t.Fatalf("record memory after human approval: %v", err)
	}
	memoryValues, err = f.memory.List(ctx, f.project.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(memoryValues) != 1 || memoryValues[0].Category() != memory.CategoryTaskHistory {
		t.Fatalf("memory after human approval = %+v, want one task history summary", memoryValues)
	}
}

func TestHumanReviewRejectionDoesNotPersistMemory(t *testing.T) {
	f := newFixture(t, fixtureOptions{humanReview: true, maxAttempts: 3})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)
	if _, err := f.service.Dispatch(ctx, f.project.ID()); err != nil {
		t.Fatal(err)
	}
	awaitingHuman, err := f.service.Review(ctx, ready.ID(), orchestrator.DecisionApprove, "awaiting human approval")
	if err != nil {
		t.Fatal(err)
	}
	fakeDone, err := awaitingHuman.Transition(task.StatusDone)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.RecordHumanReviewCompletion(ctx, fakeDone); err == nil {
		t.Fatal("Brain transition was accepted as human approval")
	}
	if _, err := f.tasks.Transition(ctx, ready.ID(), task.StatusWorking, "human rejected; rework required", f.adminActor()); err != nil {
		t.Fatal(err)
	}
	memoryValues, err := f.memory.List(ctx, f.project.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(memoryValues) != 0 {
		t.Fatalf("rejected human review left durable project memory: %+v", memoryValues)
	}
}

func TestApprovalRespectsBrainAutonomy(t *testing.T) {
	f := newFixture(t, fixtureOptions{maxAttempts: 3})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)
	if _, err := f.service.Dispatch(ctx, f.project.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.brainService.Replace(ctx, f.project.ID(), brain.CreateInput{AgentID: f.brainAgent.ID(), Autonomy: brain.AutonomySemiAuto}, project.LocalAdministratorID, "tighten autonomy"); err != nil {
		t.Fatal(err)
	}

	if _, err := f.service.Review(ctx, ready.ID(), orchestrator.DecisionApprove, "approved"); !errors.Is(err, brain.ErrHumanApprovalRequired) {
		t.Fatalf("review error = %v, want human approval requirement", err)
	}
	value, err := f.tasks.Get(ctx, ready.ID())
	if err != nil {
		t.Fatal(err)
	}
	if value.Status() != task.StatusReview {
		t.Fatalf("task status = %s, want REVIEW", value.Status())
	}
}

func TestDispatchSkipsExternalAgents(t *testing.T) {
	f := newFixture(t, fixtureOptions{maxAttempts: 3, workerRuntime: agent.RuntimeMCP})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)

	report, err := f.service.Dispatch(ctx, f.project.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != orchestrator.OutcomeSkipped {
		t.Fatalf("dispatch outcomes = %+v", report.Outcomes)
	}
	if len(f.executor.Calls()) != 0 {
		t.Fatal("external agent task was executed in-process")
	}
	value, err := f.tasks.Get(ctx, ready.ID())
	if err != nil {
		t.Fatal(err)
	}
	if value.Status() != task.StatusReady {
		t.Fatalf("external task status changed to %s", value.Status())
	}
}

func TestDispatchRequiresBrainPolicy(t *testing.T) {
	f := newFixture(t, fixtureOptions{maxAttempts: 3, brainCaps: []string{"brain.execute"}})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)

	report, err := f.service.Dispatch(ctx, f.project.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != orchestrator.OutcomeSkipped {
		t.Fatalf("dispatch outcomes = %+v", report.Outcomes)
	}
	if len(f.executor.Calls()) != 0 {
		t.Fatal("task was executed without Brain policy capability")
	}
	value, err := f.tasks.Get(ctx, ready.ID())
	if err != nil {
		t.Fatal(err)
	}
	if value.Status() != task.StatusReady {
		t.Fatalf("task status = %s, want READY", value.Status())
	}
}

func TestDuplicateClaimIsPrevented(t *testing.T) {
	f := newFixture(t, fixtureOptions{maxAttempts: 3})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)
	if _, err := f.tasks.Transition(ctx, ready.ID(), task.StatusWorking, "manual claim", f.adminActor()); err != nil {
		t.Fatal(err)
	}

	report, err := f.service.Dispatch(ctx, f.project.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 0 || len(f.executor.Calls()) != 0 {
		t.Fatalf("claimed task was dispatched again: %+v", report.Outcomes)
	}
}

func TestAttemptLimitCountsOnlyWorkerExecutions(t *testing.T) {
	f := newFixture(t, fixtureOptions{maxAttempts: 3})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)
	f.executor.errs = []error{errors.New("fail-1"), errors.New("fail-2"), errors.New("fail-3")}

	// Simulate the Brain failure-analysis Runs that the production advisor persists.
	for _, id := range []string{"analysis-1", "analysis-2"} {
		value, err := run.NewBrainAnalysisRun(id, f.project.ID(), ready.ID(), f.brainAgent.ID())
		if err != nil {
			t.Fatal(err)
		}
		value, err = value.ConfigureExecution(string(agent.RuntimeAPI), "openai-compatible", "test-model", "failure analysis")
		if err != nil {
			t.Fatal(err)
		}
		if err := f.runs.Create(ctx, value); err != nil {
			t.Fatal(err)
		}
	}

	for attempt := 1; attempt <= 3; attempt++ {
		report, err := f.service.Dispatch(ctx, f.project.ID())
		if err != nil {
			t.Fatalf("dispatch %d: %v", attempt, err)
		}
		if len(report.Outcomes) != 1 {
			t.Fatalf("dispatch %d outcomes = %+v", attempt, report.Outcomes)
		}
		want := orchestrator.OutcomeRetryScheduled
		if attempt == 3 {
			want = orchestrator.OutcomeEscalated
		}
		if report.Outcomes[0].Status != want {
			t.Fatalf("dispatch %d outcome = %s, want %s", attempt, report.Outcomes[0].Status, want)
		}
	}
	if got := len(f.executor.Calls()); got != 3 {
		t.Fatalf("worker executions = %d, want 3", got)
	}
	workerRuns, err := f.runs.List(ctx, f.project.ID(), ready.ID(), "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	workerCount := 0
	for _, value := range workerRuns {
		if value.Kind() == run.KindWorker {
			workerCount++
		}
	}
	if workerCount != 3 {
		t.Fatalf("worker runs = %d, want 3", workerCount)
	}
	blocked, err := f.tasks.Get(ctx, ready.ID())
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status() != task.StatusBlocked {
		t.Fatalf("task status = %s, want BLOCKED", blocked.Status())
	}
	if len(f.escalations.Values()) != 1 {
		t.Fatalf("escalations = %d, want 1", len(f.escalations.Values()))
	}
}

type blockingExecutor struct{ started chan struct{} }

func (b *blockingExecutor) Execute(ctx context.Context, _ runtime.Request) (runtime.Result, error) {
	close(b.started)
	<-ctx.Done()
	return runtime.Result{}, ctx.Err()
}

func TestCancellationLeavesTaskRecoverable(t *testing.T) {
	f := newFixture(t, fixtureOptions{maxAttempts: 3})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)

	blocker := &blockingExecutor{started: make(chan struct{})}
	service, err := orchestrator.New(orchestrator.Dependencies{
		Tasks: f.tasks, Runs: f.runs, Agents: f.agents, Projects: f.projects,
		Brains: f.brainRepository, Executor: blocker,
		Conversations: f.conversations, Escalations: f.escalations,
		Config: orchestrator.Config{MaxAttempts: 3},
	})
	if err != nil {
		t.Fatal(err)
	}

	requestCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var report orchestrator.DispatchReport
	var dispatchErr error
	go func() {
		report, dispatchErr = service.Dispatch(requestCtx, f.project.ID())
		close(done)
	}()

	<-blocker.started
	cancel()
	<-done

	if dispatchErr != nil {
		t.Fatalf("dispatch error = %v", dispatchErr)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != orchestrator.OutcomeRetryScheduled {
		t.Fatalf("outcomes = %+v", report.Outcomes)
	}
	value, err := f.tasks.Get(ctx, ready.ID())
	if err != nil {
		t.Fatal(err)
	}
	if value.Status() != task.StatusReady {
		t.Fatalf("task status = %s, want READY (retry scheduled), not orphaned in WORKING", value.Status())
	}
	runs, err := f.runs.List(ctx, f.project.ID(), ready.ID(), "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status() != run.StatusCancelled {
		t.Fatalf("runs = %+v, want one CANCELLED run", runs)
	}
}

type delayedSuccessExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (d *delayedSuccessExecutor) Execute(context.Context, runtime.Request) (runtime.Result, error) {
	close(d.started)
	<-d.release
	return runtime.Result{Output: "completed after cleanup timeout"}, nil
}

func TestSuccessfulExecutionGetsFreshCleanupContext(t *testing.T) {
	const cleanupTimeout = 500 * time.Millisecond
	f := newFixture(t, fixtureOptions{maxAttempts: 3})
	ctx := context.Background()
	ready := f.readyTask(t, ctx)
	executor := &delayedSuccessExecutor{started: make(chan struct{}), release: make(chan struct{})}
	service, err := orchestrator.New(orchestrator.Dependencies{
		Tasks: f.tasks, Runs: f.runs, Agents: f.agents, Projects: f.projects,
		Brains: f.brainRepository, Executor: executor,
		Conversations: f.conversations, Escalations: f.escalations,
		Config: orchestrator.Config{MaxAttempts: 3, ExecutionTimeout: time.Second, CleanupTimeout: cleanupTimeout},
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var report orchestrator.DispatchReport
	var dispatchErr error
	go func() {
		report, dispatchErr = service.Dispatch(ctx, f.project.ID())
		close(done)
	}()
	select {
	case <-executor.started:
	case <-done:
		t.Fatalf("dispatch returned before worker execution: outcomes=%+v error=%v", report.Outcomes, dispatchErr)
	}
	time.Sleep(2 * cleanupTimeout)
	close(executor.release)
	<-done

	if dispatchErr != nil {
		t.Fatalf("dispatch error = %v", dispatchErr)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != orchestrator.OutcomeSucceeded {
		t.Fatalf("outcomes = %+v, want one successful execution", report.Outcomes)
	}
	values, err := f.runs.List(ctx, f.project.ID(), ready.ID(), "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Status() != run.StatusSuccess {
		t.Fatalf("runs = %+v, want one SUCCESS run", values)
	}
	updated, err := f.tasks.Get(ctx, ready.ID())
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status() != task.StatusReview {
		t.Fatalf("task status = %s, want REVIEW", updated.Status())
	}
}

func assertTransitions(t *testing.T, database *sql.DB, taskID string, expected [][2]string) {
	t.Helper()
	rows, err := database.Query(`SELECT from_status, to_status FROM audit_events WHERE task_id=? AND event_type='task.status.changed' ORDER BY sequence`, taskID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var actual [][2]string
	for rows.Next() {
		var from, to string
		if err := rows.Scan(&from, &to); err != nil {
			t.Fatal(err)
		}
		actual = append(actual, [2]string{from, to})
	}
	if len(actual) < len(expected) {
		t.Fatalf("transitions = %+v, want at least %+v", actual, expected)
	}
	offset := len(actual) - len(expected)
	for index, want := range expected {
		if actual[offset+index] != want {
			t.Fatalf("transitions = %+v, want tail %+v", actual, expected)
		}
	}
}
