// Package orchestrator coordinates Brain-supervised task execution, review, retries, and escalation.
package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/audit"
	"boAIrd/internal/brain"
	"boAIrd/internal/conversation"
	"boAIrd/internal/logging"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/runtime"
	"boAIrd/internal/task"
)

var (
	ErrInvalidRequest     = errors.New("invalid orchestration request")
	ErrProjectUnavailable = errors.New("project cannot be orchestrated")
	ErrTaskNotInReview    = errors.New("task is not awaiting review")
)

const (
	defaultMaxAttempts      = 3
	maximumMaxAttempts      = 10
	defaultMaxTasksDispatch = 10
	maximumMaxTasksDispatch = 100
	defaultExecutionTimeout = 5 * time.Minute
	maximumExecutionTimeout = time.Hour
	defaultAdvisorTimeout   = 45 * time.Second
	finalizeTimeout         = 15 * time.Second
	workerMaxTokens         = 2048
	maximumInputBytes       = 16 << 10
)

// Config bounds scheduling, retries, and executor runtime.
type Config struct {
	MaxAttempts         int
	MaxTasksPerDispatch int
	ExecutionTimeout    time.Duration
}

// TaskManager is the policy-enforcing task service used for every transition.
type TaskManager interface {
	Get(context.Context, string) (task.Task, error)
	List(context.Context, string, task.Status, string, int, int) ([]task.Task, error)
	Transition(context.Context, string, task.Status, string, task.Actor) (task.Task, error)
}

// RunRepository persists the execution record of each attempt.
type RunRepository interface {
	Create(context.Context, run.Run) error
	Update(context.Context, run.Run) error
	CountWorkerAttempts(context.Context, string, string) (int, error)
}

type AgentReader interface {
	Get(context.Context, string) (agent.Agent, error)
}
type ProjectReader interface {
	Get(context.Context, string) (project.Project, error)
}
type BrainReader interface {
	GetActiveByProject(context.Context, string) (brain.Brain, error)
}

// ConversationWriter records Brain instructions for a retried task.
type ConversationWriter interface {
	Append(context.Context, string, conversation.SenderType, string, conversation.MessageInput) (conversation.Message, error)
}

// FailureAdvisor lets the project Brain analyze a worker failure before retrying.
type FailureAdvisor interface {
	Analyze(context.Context, string, string, string) (string, error)
}

// Dependencies wires the orchestrator; conversations and escalations are optional.
type Dependencies struct {
	Tasks         TaskManager
	Runs          RunRepository
	Agents        AgentReader
	Projects      ProjectReader
	Brains        BrainReader
	Executor      runtime.Executor
	Conversations ConversationWriter
	Escalations   audit.Recorder
	Advisor       FailureAdvisor
	Config        Config
}

// Execution statuses reported for one dispatched task.
const (
	OutcomeSucceeded      = "SUCCEEDED"
	OutcomeRetryScheduled = "RETRY_SCHEDULED"
	OutcomeEscalated      = "ESCALATED"
	OutcomeSkipped        = "SKIPPED"
)

// ExecutionOutcome reports what happened to one scheduled task.
type ExecutionOutcome struct {
	TaskID string `json:"task_id"`
	RunID  string `json:"run_id,omitempty"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// DispatchReport summarizes one scheduling pass.
type DispatchReport struct {
	Outcomes []ExecutionOutcome `json:"outcomes"`
}

// ReviewDecision is the Brain's verdict for submitted work.
type ReviewDecision string

const (
	DecisionApprove ReviewDecision = "APPROVE"
	DecisionRework  ReviewDecision = "REWORK"
)

type Service struct {
	tasks         TaskManager
	runs          RunRepository
	agents        AgentReader
	projects      ProjectReader
	brains        BrainReader
	executor      runtime.Executor
	conversations ConversationWriter
	escalations   audit.Recorder
	advisor       FailureAdvisor
	config        Config
}

func New(dependencies Dependencies) (*Service, error) {
	if dependencies.Tasks == nil || dependencies.Runs == nil || dependencies.Agents == nil ||
		dependencies.Projects == nil || dependencies.Brains == nil || dependencies.Executor == nil {
		return nil, errors.New("task, run, agent, project, Brain, and executor dependencies are required")
	}
	config := dependencies.Config
	if config.MaxAttempts == 0 {
		config.MaxAttempts = defaultMaxAttempts
	}
	if config.MaxTasksPerDispatch == 0 {
		config.MaxTasksPerDispatch = defaultMaxTasksDispatch
	}
	if config.ExecutionTimeout == 0 {
		config.ExecutionTimeout = defaultExecutionTimeout
	}
	if config.MaxAttempts < 1 || config.MaxAttempts > maximumMaxAttempts {
		return nil, fmt.Errorf("%w: max attempts must be between 1 and %d", ErrInvalidRequest, maximumMaxAttempts)
	}
	if config.MaxTasksPerDispatch < 1 || config.MaxTasksPerDispatch > maximumMaxTasksDispatch {
		return nil, fmt.Errorf("%w: max tasks per dispatch must be between 1 and %d", ErrInvalidRequest, maximumMaxTasksDispatch)
	}
	if config.ExecutionTimeout < 0 || config.ExecutionTimeout > maximumExecutionTimeout {
		return nil, fmt.Errorf("%w: execution timeout must be positive and at most %s", ErrInvalidRequest, maximumExecutionTimeout)
	}
	return &Service{
		tasks: dependencies.Tasks, runs: dependencies.Runs, agents: dependencies.Agents, projects: dependencies.Projects,
		brains: dependencies.Brains, executor: dependencies.Executor, conversations: dependencies.Conversations,
		escalations: dependencies.Escalations, advisor: dependencies.Advisor, config: config,
	}, nil
}

// Dispatch claims READY tasks assigned to push-executable Agents and executes them once each.
func (s *Service) Dispatch(ctx context.Context, projectID string) (DispatchReport, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return DispatchReport{}, fmt.Errorf("%w: project ID is required", ErrInvalidRequest)
	}
	p, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return DispatchReport{}, err
	}
	if p.Status() == project.StatusArchived {
		return DispatchReport{}, ErrProjectUnavailable
	}
	b, err := s.brains.GetActiveByProject(ctx, projectID)
	if err != nil {
		return DispatchReport{}, err
	}
	if _, err := s.authorizeBrain(ctx, b, p); err != nil {
		return DispatchReport{}, err
	}
	brainActor := task.Actor{Type: task.ActorBrain, ID: b.AgentID()}
	values, err := s.tasks.List(ctx, projectID, task.StatusReady, "", s.config.MaxTasksPerDispatch, 0)
	if err != nil {
		return DispatchReport{}, err
	}
	report := DispatchReport{Outcomes: make([]ExecutionOutcome, 0, len(values))}
	var failures []error
	for _, value := range values {
		outcome, err := s.dispatchTask(ctx, p, b, brainActor, value)
		report.Outcomes = append(report.Outcomes, outcome)
		if err != nil {
			failures = append(failures, fmt.Errorf("dispatch task %s: %w", value.ID(), err))
		}
	}
	return report, errors.Join(failures...)
}

func (s *Service) dispatchTask(ctx context.Context, p project.Project, b brain.Brain, brainActor task.Actor, work task.Task) (ExecutionOutcome, error) {
	if work.AssignedTo() == "" {
		return skipped(work, "task has no assigned agent"), nil
	}
	a, err := s.agents.Get(ctx, work.AssignedTo())
	if err != nil {
		return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "assigned agent is unavailable"}, err
	}
	if a.Runtime() != agent.RuntimeAPI {
		return skipped(work, "external agents claim work through MCP"), nil
	}
	if !hasCapability(a, "task.transition") {
		return skipped(work, "assigned agent lacks the task.transition capability"), nil
	}
	attempts, err := s.attempts(ctx, work.ProjectID(), work.ID())
	if err != nil {
		return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "execution history is unavailable"}, err
	}
	if attempts >= s.config.MaxAttempts {
		claimed, err := s.tasks.Transition(ctx, work.ID(), task.StatusWorking,
			fmt.Sprintf("orchestrator blocked task: retry limit %d reached", s.config.MaxAttempts), brainActor)
		if err != nil {
			return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "retry limit could not be recorded"}, err
		}
		if _, err := s.tasks.Transition(ctx, claimed.ID(), task.StatusBlocked,
			fmt.Sprintf("retry limit %d reached without a successful attempt", s.config.MaxAttempts), brainActor); err != nil {
			return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "retry limit could not be recorded"}, err
		}
		escalateErr := s.escalate(ctx, p, work, "", a.ID(), attempts, task.StatusBlocked, "retry limit reached before dispatch")
		return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "retry limit reached before dispatch"}, escalateErr
	}
	claimed, err := s.tasks.Transition(ctx, work.ID(), task.StatusWorking,
		fmt.Sprintf("orchestrator dispatch: attempt %d of %d", attempts+1, s.config.MaxAttempts), brainActor)
	if err != nil {
		switch {
		case errors.Is(err, task.ErrConflict):
			return skipped(work, "task was claimed concurrently"), nil
		case errors.Is(err, task.ErrForbidden):
			return skipped(work, "dispatch is not authorized by project policy"), nil
		default:
			return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "task could not be claimed"}, err
		}
	}
	return s.execute(ctx, p, b, claimed, a, attempts+1, "")
}

// Review applies the Brain verdict to submitted work without bypassing Human Review.
func (s *Service) Review(ctx context.Context, taskID string, decision ReviewDecision, reason string) (task.Task, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return task.Task{}, fmt.Errorf("%w: task ID is required", ErrInvalidRequest)
	}
	work, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return task.Task{}, err
	}
	if work.Status() != task.StatusReview {
		return task.Task{}, ErrTaskNotInReview
	}
	p, err := s.projects.Get(ctx, work.ProjectID())
	if err != nil {
		return task.Task{}, err
	}
	b, err := s.brains.GetActiveByProject(ctx, work.ProjectID())
	if err != nil {
		return task.Task{}, err
	}
	if _, err := s.authorizeBrain(ctx, b, p); err != nil {
		return task.Task{}, err
	}
	brainActor := task.Actor{Type: task.ActorBrain, ID: b.AgentID()}
	reason = truncate(logging.RedactSensitiveText(strings.TrimSpace(reason)), 1500)
	switch decision {
	case DecisionApprove:
		summary := "Brain approved work"
		if reason != "" {
			summary += ": " + reason
		}
		if work.HumanReview() {
			return s.tasks.Transition(ctx, work.ID(), task.StatusHumanReview, summary+"; human review required", brainActor)
		}
		return s.tasks.Transition(ctx, work.ID(), task.StatusDone, summary, brainActor)
	case DecisionRework:
		if reason == "" {
			return task.Task{}, fmt.Errorf("%w: rework reason is required", ErrInvalidRequest)
		}
		if work.AssignedTo() == "" {
			return task.Task{}, fmt.Errorf("%w: task has no assigned agent", ErrInvalidRequest)
		}
		attempts, err := s.attempts(ctx, work.ProjectID(), work.ID())
		if err != nil {
			return task.Task{}, err
		}
		if attempts >= s.config.MaxAttempts {
			failed, err := s.tasks.Transition(ctx, work.ID(), task.StatusFailed,
				"Brain rejected work after the maximum number of attempts: "+reason, brainActor)
			if err != nil {
				return task.Task{}, err
			}
			return failed, s.escalate(ctx, p, failed, "", work.AssignedTo(), attempts, task.StatusFailed, "review rejected work after maximum attempts")
		}
		a, err := s.agents.Get(ctx, work.AssignedTo())
		if err != nil {
			return task.Task{}, err
		}
		reworked, err := s.tasks.Transition(ctx, work.ID(), task.StatusWorking, "Brain requested rework: "+reason, brainActor)
		if err != nil {
			return task.Task{}, err
		}
		if a.Runtime() != agent.RuntimeAPI {
			return reworked, nil
		}
		if _, err := s.execute(ctx, p, b, reworked, a, attempts+1, reason); err != nil {
			return task.Task{}, err
		}
		return s.tasks.Get(ctx, work.ID())
	default:
		return task.Task{}, fmt.Errorf("%w: review decision must be APPROVE or REWORK", ErrInvalidRequest)
	}
}

func (s *Service) execute(ctx context.Context, p project.Project, b brain.Brain, work task.Task, a agent.Agent, attempt int, instructions string) (ExecutionOutcome, error) {
	input := buildInput(work, attempt, s.config.MaxAttempts, instructions)
	id, err := newID()
	if err != nil {
		return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "run ID could not be generated"}, err
	}
	value, err := run.New(id, work.ProjectID(), work.ID(), a.ID())
	if err != nil {
		return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "run could not be created"}, err
	}
	value, err = value.ConfigureExecution(string(a.Runtime()), a.Provider(), a.Model(), input)
	if err != nil {
		return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "run could not be configured"}, err
	}
	persistCtx, cancelPersist := detached(ctx)
	defer cancelPersist()
	if err := s.runs.Create(persistCtx, value); err != nil {
		return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "run could not be persisted"}, err
	}
	value, err = value.Transition(run.StatusRunning)
	if err != nil {
		return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "run could not be started"}, err
	}
	if err := s.runs.Update(persistCtx, value); err != nil {
		return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "run could not be started"}, err
	}

	execCtx, cancel := context.WithTimeout(ctx, s.config.ExecutionTimeout)
	defer cancel()
	result, executionErr := s.executor.Execute(execCtx, runtime.Request{
		RunID: value.ID(), ProjectID: work.ProjectID(), TaskID: work.ID(), AgentID: a.ID(), Input: input, MaxTokens: workerMaxTokens,
	})
	if executionErr != nil {
		return s.failExecution(ctx, p, b, work, a, value, attempt, executionErr)
	}
	value, err = value.SetOutput(result.Output)
	if err == nil {
		value, err = value.Transition(run.StatusSuccess)
	}
	if err != nil {
		return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "worker output could not be recorded"}, err
	}
	if err := s.runs.Update(persistCtx, value); err != nil {
		return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeEscalated, Reason: "worker output could not be recorded"}, err
	}
	workerActor := task.Actor{Type: task.ActorAgent, ID: a.ID()}
	if _, err := s.tasks.Transition(persistCtx, work.ID(), task.StatusReview, fmt.Sprintf("worker completed run %s", value.ID()), workerActor); err != nil {
		escalateErr := s.escalate(persistCtx, p, work, value.ID(), a.ID(), attempt, task.StatusWorking, "worker output could not be submitted for review")
		return ExecutionOutcome{TaskID: work.ID(), RunID: value.ID(), Status: OutcomeEscalated, Reason: "worker output could not be submitted for review"}, errors.Join(err, escalateErr)
	}
	return ExecutionOutcome{TaskID: work.ID(), RunID: value.ID(), Status: OutcomeSucceeded}, nil
}

func (s *Service) failExecution(ctx context.Context, p project.Project, b brain.Brain, work task.Task, a agent.Agent, value run.Run, attempt int, cause error) (ExecutionOutcome, error) {
	status, summary := run.StatusFailed, "worker execution failed"
	if errors.Is(cause, context.Canceled) {
		status, summary = run.StatusCancelled, "worker execution cancelled"
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		status, summary = run.StatusCancelled, "worker execution timed out"
	}
	value, _ = value.SetError(summary)
	value, _ = value.Transition(status)
	persistCtx, cancelPersist := detached(ctx)
	defer cancelPersist()
	if err := s.runs.Update(persistCtx, value); err != nil {
		return ExecutionOutcome{TaskID: work.ID(), RunID: value.ID(), Status: OutcomeEscalated, Reason: summary}, err
	}
	brainActor := task.Actor{Type: task.ActorBrain, ID: b.AgentID()}
	reason := fmt.Sprintf("%s on attempt %d of %d", summary, attempt, s.config.MaxAttempts)
	if _, err := s.tasks.Transition(persistCtx, work.ID(), task.StatusBlocked, reason, brainActor); err != nil {
		escalateErr := s.escalate(persistCtx, p, work, value.ID(), a.ID(), attempt, work.Status(), "failure could not be recorded as BLOCKED")
		return ExecutionOutcome{TaskID: work.ID(), RunID: value.ID(), Status: OutcomeEscalated, Reason: "failure could not be recorded"}, errors.Join(err, escalateErr)
	}
	if attempt >= s.config.MaxAttempts {
		escalateErr := s.escalate(persistCtx, p, work, value.ID(), a.ID(), attempt, task.StatusBlocked, summary)
		return ExecutionOutcome{TaskID: work.ID(), RunID: value.ID(), Status: OutcomeEscalated, Reason: summary}, escalateErr
	}
	instructions := s.advise(persistCtx, work, attempt, summary)
	if s.conversations != nil {
		content := fmt.Sprintf("Attempt %d of %d failed: %s\n\nRetry instructions:\n%s", attempt, s.config.MaxAttempts, summary, instructions)
		if _, err := s.conversations.Append(persistCtx, work.ID(), conversation.SenderBrain, b.AgentID(), conversation.MessageInput{
			Content: content, ContentType: "text/plain", Metadata: []byte(`{}`),
		}); err != nil {
			slog.ErrorContext(persistCtx, "record Brain retry instructions", "task_id", work.ID(), "error", err)
		}
	}
	if _, err := s.tasks.Transition(persistCtx, work.ID(), task.StatusReady,
		fmt.Sprintf("Brain scheduled retry %d of %d: %s", attempt+1, s.config.MaxAttempts, truncate(instructions, 1200)), brainActor); err != nil {
		escalateErr := s.escalate(persistCtx, p, work, value.ID(), a.ID(), attempt, task.StatusBlocked, "retry could not be scheduled")
		return ExecutionOutcome{TaskID: work.ID(), RunID: value.ID(), Status: OutcomeEscalated, Reason: "retry could not be scheduled"}, errors.Join(err, escalateErr)
	}
	return ExecutionOutcome{TaskID: work.ID(), RunID: value.ID(), Status: OutcomeRetryScheduled, Reason: summary}, nil
}

func (s *Service) advise(ctx context.Context, work task.Task, attempt int, failure string) string {
	const fallback = "Retry the task and address the reported failure while keeping the deliverable within the assigned scope."
	if s.advisor == nil {
		return fallback
	}
	objective := truncate(fmt.Sprintf(
		"A worker task failed on attempt %d of %d. Return concise retry instructions for the worker.\nTask: %s\nDescription: %s\nFailure: %s",
		attempt, s.config.MaxAttempts, work.Title(), work.Description(), failure), maximumInputBytes)
	adviceCtx, cancel := context.WithTimeout(ctx, defaultAdvisorTimeout)
	defer cancel()
	advice, err := s.advisor.Analyze(adviceCtx, work.ProjectID(), work.ID(), objective)
	if err != nil {
		slog.WarnContext(ctx, "Brain failure analysis unavailable", "task_id", work.ID(), "error", err)
		return fallback
	}
	if strings.TrimSpace(advice) == "" {
		return fallback
	}
	return truncate(logging.RedactSensitiveText(strings.TrimSpace(advice)), 1200)
}

func (s *Service) attempts(ctx context.Context, projectID, taskID string) (int, error) {
	return s.runs.CountWorkerAttempts(ctx, projectID, taskID)
}

func (s *Service) escalate(ctx context.Context, p project.Project, work task.Task, runID, agentID string, attempts int, status task.Status, reason string) error {
	reason = truncate(logging.RedactSensitiveText(strings.TrimSpace(reason)), 2000)
	slog.WarnContext(ctx, "orchestration requires administrator action", "project_id", p.ID(), "task_id", work.ID(), "agent_id", agentID, "attempts", attempts)
	if s.escalations == nil {
		return nil
	}
	return s.escalations.RecordEscalation(ctx, audit.Escalation{
		ProjectID: p.ID(), TaskID: work.ID(), RunID: runID, AgentID: agentID, Attempts: attempts, Status: string(status), Reason: reason,
	})
}

func skipped(work task.Task, reason string) ExecutionOutcome {
	return ExecutionOutcome{TaskID: work.ID(), Status: OutcomeSkipped, Reason: reason}
}

// authorizeBrain fails closed unless the configured Brain is active, matches the
// project, and its Agent holds the brain.execute capability.
func (s *Service) authorizeBrain(ctx context.Context, b brain.Brain, p project.Project) (agent.Agent, error) {
	a, err := s.agents.Get(ctx, b.AgentID())
	if err != nil {
		return agent.Agent{}, err
	}
	if err := b.AuthorizeAction(brain.ActionPlan, p, a); err != nil {
		return agent.Agent{}, err
	}
	return a, nil
}

// detached returns a bounded cleanup context that survives request cancellation.
func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
}

func hasCapability(a agent.Agent, required string) bool {
	for _, value := range a.Capabilities() {
		if string(value) == required {
			return true
		}
	}
	return false
}

func buildInput(work task.Task, attempt, maximum int, instructions string) string {
	var builder strings.Builder
	builder.WriteString("Task: ")
	builder.WriteString(work.Title())
	builder.WriteString("\n")
	if description := strings.TrimSpace(work.Description()); description != "" {
		builder.WriteString("Description:\n")
		builder.WriteString(description)
		builder.WriteString("\n")
	}
	fmt.Fprintf(&builder, "Priority: %s\nAttempt: %d of %d\n", work.Priority(), attempt, maximum)
	if instructions = strings.TrimSpace(instructions); instructions != "" {
		builder.WriteString("Review instructions:\n")
		builder.WriteString(instructions)
		builder.WriteString("\n")
	}
	builder.WriteString("Return the complete deliverable for this task as text.")
	return truncate(builder.String(), maximumInputBytes)
}

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
