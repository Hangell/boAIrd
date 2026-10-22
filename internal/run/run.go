// Package run defines provider-independent execution records.
package run

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"boAIrd/internal/logging"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusRunning   Status = "RUNNING"
	StatusSuccess   Status = "SUCCESS"
	StatusFailed    Status = "FAILED"
	StatusCancelled Status = "CANCELLED"
)

var (
	ErrInvalidTransition = errors.New("invalid run status transition")
	ErrNotFound          = errors.New("run not found")
)

type Run struct {
	id         string
	projectID  string
	taskID     string
	agentID    string
	status     Status
	startedAt  time.Time
	finishedAt time.Time
	runtime    string
	provider   string
	model      string
	input      string
	output     string
	errorText  string
	toolsUsed  []string
}

type Snapshot struct {
	ID         string
	ProjectID  string
	TaskID     string
	AgentID    string
	Status     Status
	StartedAt  time.Time
	FinishedAt time.Time
	Runtime    string
	Provider   string
	Model      string
	Input      string
	Output     string
	Error      string
	ToolsUsed  []string
}

type AuditEvent struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	TaskID       string    `json:"task_id,omitempty"`
	ActorType    string    `json:"actor_type"`
	ActorID      string    `json:"actor_id"`
	EventType    string    `json:"event_type"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	FromStatus   string    `json:"from_status,omitempty"`
	ToStatus     string    `json:"to_status,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func New(id, projectID, taskID, agentID string) (Run, error) {
	if strings.TrimSpace(taskID) == "" {
		return Run{}, errors.New("task ID is required")
	}
	value, err := NewBrainRun(id, projectID, agentID)
	if err != nil {
		return Run{}, err
	}
	value.taskID = strings.TrimSpace(taskID)
	return value, nil
}

func NewBrainRun(id, projectID, agentID string) (Run, error) {
	for _, reference := range []struct{ name, value string }{
		{"run ID", id}, {"project ID", projectID}, {"agent ID", agentID},
	} {
		if strings.TrimSpace(reference.value) == "" {
			return Run{}, fmt.Errorf("%s is required", reference.name)
		}
	}
	return Run{id: strings.TrimSpace(id), projectID: strings.TrimSpace(projectID), agentID: strings.TrimSpace(agentID), status: StatusPending}, nil
}

func (r Run) ID() string            { return r.id }
func (r Run) ProjectID() string     { return r.projectID }
func (r Run) TaskID() string        { return r.taskID }
func (r Run) AgentID() string       { return r.agentID }
func (r Run) Status() Status        { return r.status }
func (r Run) StartedAt() time.Time  { return r.startedAt }
func (r Run) FinishedAt() time.Time { return r.finishedAt }
func (r Run) UsedTools() []string   { return append([]string(nil), r.toolsUsed...) }
func (r Run) Runtime() string       { return r.runtime }
func (r Run) Provider() string      { return r.provider }
func (r Run) Model() string         { return r.model }
func (r Run) Input() string         { return r.input }
func (r Run) Output() string        { return r.output }
func (r Run) Error() string         { return r.errorText }

func (r Run) Snapshot() Snapshot {
	return Snapshot{
		ID: r.id, ProjectID: r.projectID, TaskID: r.taskID, AgentID: r.agentID, Status: r.status,
		StartedAt: r.startedAt, FinishedAt: r.finishedAt, Runtime: r.runtime, Provider: r.provider,
		Model: r.model, Input: r.input, Output: r.output, Error: r.errorText, ToolsUsed: append([]string(nil), r.toolsUsed...),
	}
}

func Restore(snapshot Snapshot) (Run, error) {
	value, err := NewBrainRun(snapshot.ID, snapshot.ProjectID, snapshot.AgentID)
	if err != nil {
		return Run{}, err
	}
	if strings.TrimSpace(snapshot.TaskID) != "" {
		value.taskID = strings.TrimSpace(snapshot.TaskID)
	}
	if !validStatus(snapshot.Status) {
		return Run{}, fmt.Errorf("unknown run status %q", snapshot.Status)
	}
	if snapshot.Status == StatusRunning && snapshot.StartedAt.IsZero() {
		return Run{}, errors.New("running run requires a start time")
	}
	if (snapshot.Status == StatusSuccess || snapshot.Status == StatusFailed || snapshot.Status == StatusCancelled) && snapshot.FinishedAt.IsZero() {
		return Run{}, errors.New("terminal run requires a finish time")
	}
	value.status, value.startedAt, value.finishedAt = snapshot.Status, snapshot.StartedAt, snapshot.FinishedAt
	value.runtime, value.provider, value.model = snapshot.Runtime, snapshot.Provider, snapshot.Model
	value.input, value.output, value.errorText = logging.RedactSensitiveText(snapshot.Input), logging.RedactSensitiveText(snapshot.Output), logging.RedactSensitiveText(snapshot.Error)
	value.toolsUsed = make([]string, len(snapshot.ToolsUsed))
	for i, tool := range snapshot.ToolsUsed {
		value.toolsUsed[i] = logging.RedactSensitiveText(tool)
	}
	return value, nil
}

func (r Run) ConfigureExecution(runtime, provider, model, input string) (Run, error) {
	if r.status != StatusPending {
		return Run{}, errors.New("run configuration is immutable after execution starts")
	}
	if strings.TrimSpace(runtime) == "" || strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" || strings.TrimSpace(input) == "" {
		return Run{}, errors.New("run runtime, provider, model, and input are required")
	}
	r.runtime, r.provider, r.model, r.input = strings.TrimSpace(runtime), strings.TrimSpace(provider), strings.TrimSpace(model), logging.RedactSensitiveText(input)
	return r, nil
}

func (r Run) SetOutput(output string) (Run, error) {
	if r.status != StatusRunning {
		return Run{}, errors.New("run output can only be stored while running")
	}
	if strings.TrimSpace(output) == "" {
		return Run{}, errors.New("run output is required")
	}
	r.output = logging.RedactSensitiveText(output)
	return r, nil
}

func (r Run) SetError(message string) (Run, error) {
	if r.status != StatusRunning {
		return Run{}, errors.New("run error can only be stored while running")
	}
	if strings.TrimSpace(message) == "" {
		return Run{}, errors.New("run error message is required")
	}
	r.errorText = logging.RedactSensitiveText(strings.TrimSpace(message))
	return r, nil
}

func (r Run) RecordToolUse(name string) (Run, error) {
	if strings.TrimSpace(name) == "" {
		return Run{}, errors.New("tool name is required")
	}
	if r.status != StatusRunning {
		return Run{}, fmt.Errorf("tools can only be recorded while a run is %s", StatusRunning)
	}
	r.toolsUsed = append(append([]string(nil), r.toolsUsed...), logging.RedactSensitiveText(name))
	return r, nil
}

func (r Run) Transition(to Status) (Run, error) {
	if !validStatus(to) {
		return Run{}, fmt.Errorf("unknown run status %q", to)
	}
	if !((r.status == StatusPending && (to == StatusRunning || to == StatusCancelled)) ||
		(r.status == StatusRunning && (to == StatusSuccess || to == StatusFailed || to == StatusCancelled))) {
		return Run{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, r.status, to)
	}
	now := time.Now().UTC()
	if to == StatusRunning {
		r.startedAt = now
	}
	if to == StatusSuccess || to == StatusFailed || to == StatusCancelled {
		r.finishedAt = now
	}
	r.status = to
	return r, nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusPending, StatusRunning, StatusSuccess, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}
