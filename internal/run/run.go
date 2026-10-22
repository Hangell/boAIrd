// Package run defines provider-independent execution records.
package run

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusRunning   Status = "RUNNING"
	StatusSuccess   Status = "SUCCESS"
	StatusFailed    Status = "FAILED"
	StatusCancelled Status = "CANCELLED"
)

var ErrInvalidTransition = errors.New("invalid run status transition")

type Run struct {
	id         string
	projectID  string
	taskID     string
	agentID    string
	status     Status
	startedAt  time.Time
	finishedAt time.Time

	Runtime   string
	Provider  string
	Model     string
	InputRef  string
	OutputRef string
	Error     string
	ToolsUsed []string
}

func New(id, projectID, taskID, agentID string) (Run, error) {
	for _, reference := range []struct{ name, value string }{
		{"run ID", id}, {"project ID", projectID}, {"task ID", taskID}, {"agent ID", agentID},
	} {
		if strings.TrimSpace(reference.value) == "" {
			return Run{}, fmt.Errorf("%s is required", reference.name)
		}
	}
	return Run{id: id, projectID: projectID, taskID: taskID, agentID: agentID, status: StatusPending}, nil
}

func (r Run) ID() string            { return r.id }
func (r Run) ProjectID() string     { return r.projectID }
func (r Run) TaskID() string        { return r.taskID }
func (r Run) AgentID() string       { return r.agentID }
func (r Run) Status() Status        { return r.status }
func (r Run) StartedAt() time.Time  { return r.startedAt }
func (r Run) FinishedAt() time.Time { return r.finishedAt }
func (r Run) UsedTools() []string   { return append([]string(nil), r.ToolsUsed...) }

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
