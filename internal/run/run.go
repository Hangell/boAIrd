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
	ID         string
	ProjectID  string
	TaskID     string
	AgentID    string
	Runtime    string
	Provider   string
	Model      string
	InputRef   string
	OutputRef  string
	StartedAt  time.Time
	FinishedAt time.Time
	Error      string
	ToolsUsed  []string
	Status     Status
}

func New(id, projectID, taskID, agentID string) (Run, error) {
	for _, reference := range []struct{ name, value string }{
		{"run ID", id}, {"project ID", projectID}, {"task ID", taskID}, {"agent ID", agentID},
	} {
		if strings.TrimSpace(reference.value) == "" {
			return Run{}, fmt.Errorf("%s is required", reference.name)
		}
	}
	return Run{ID: id, ProjectID: projectID, TaskID: taskID, AgentID: agentID, Status: StatusPending}, nil
}

func (r Run) Transition(to Status) (Run, error) {
	if !validStatus(to) {
		return Run{}, fmt.Errorf("unknown run status %q", to)
	}
	if !((r.Status == StatusPending && (to == StatusRunning || to == StatusCancelled)) ||
		(r.Status == StatusRunning && (to == StatusSuccess || to == StatusFailed || to == StatusCancelled))) {
		return Run{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, r.Status, to)
	}
	now := time.Now().UTC()
	if to == StatusRunning {
		r.StartedAt = now
	}
	if to == StatusSuccess || to == StatusFailed || to == StatusCancelled {
		r.FinishedAt = now
	}
	r.Status = to
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
