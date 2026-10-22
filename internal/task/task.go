// Package task defines provider-independent work items and their lifecycle.
package task

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusBacklog     Status = "BACKLOG"
	StatusReady       Status = "READY"
	StatusWorking     Status = "WORKING"
	StatusReview      Status = "REVIEW"
	StatusHumanReview Status = "HUMAN_REVIEW"
	StatusBlocked     Status = "BLOCKED"
	StatusFailed      Status = "FAILED"
	StatusDone        Status = "DONE"
	StatusCancelled   Status = "CANCELLED"
)

var ErrInvalidTransition = errors.New("invalid task status transition")

type Task struct {
	id          string
	projectID   string
	title       string
	description string
	status      Status
	priority    string
	createdBy   string
	assignedTo  string
	humanReview bool
	parentID    string
	createdAt   time.Time
	updatedAt   time.Time
}

func New(id, projectID, title, description, createdBy string) (Task, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(projectID) == "" ||
		strings.TrimSpace(title) == "" || strings.TrimSpace(createdBy) == "" {
		return Task{}, errors.New("task ID, project ID, title, and creator ID are required")
	}
	now := time.Now().UTC()
	return Task{
		id: id, projectID: projectID, title: title, description: description,
		status: StatusBacklog, createdBy: createdBy, createdAt: now, updatedAt: now,
	}, nil
}

func (t Task) ID() string           { return t.id }
func (t Task) ProjectID() string    { return t.projectID }
func (t Task) Title() string        { return t.title }
func (t Task) Description() string  { return t.description }
func (t Task) Status() Status       { return t.status }
func (t Task) Priority() string     { return t.priority }
func (t Task) CreatedBy() string    { return t.createdBy }
func (t Task) AssignedTo() string   { return t.assignedTo }
func (t Task) HumanReview() bool    { return t.humanReview }
func (t Task) ParentID() string     { return t.parentID }
func (t Task) CreatedAt() time.Time { return t.createdAt }
func (t Task) UpdatedAt() time.Time { return t.updatedAt }

func (t Task) Assign(agentID string) (Task, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return Task{}, errors.New("agent ID is required")
	}
	t.assignedTo = agentID
	t.updatedAt = time.Now().UTC()
	return t, nil
}

func (t Task) Unassign() Task {
	t.assignedTo = ""
	t.updatedAt = time.Now().UTC()
	return t
}

func (t Task) SetParent(parentID string) (Task, error) {
	parentID = strings.TrimSpace(parentID)
	if parentID != "" && parentID == t.id {
		return Task{}, errors.New("task cannot be its own parent")
	}
	t.parentID = parentID
	t.updatedAt = time.Now().UTC()
	return t, nil
}

func (t Task) SetReferences(assignedTo, parentID string) (Task, error) {
	assignedTo = strings.TrimSpace(assignedTo)
	parentID = strings.TrimSpace(parentID)
	if parentID != "" && parentID == t.id {
		return Task{}, errors.New("task cannot be its own parent")
	}
	t.assignedTo = assignedTo
	t.parentID = parentID
	t.updatedAt = time.Now().UTC()
	return t, nil
}

func (t Task) RequireHumanReview() (Task, error) {
	if t.status == StatusDone || t.status == StatusCancelled {
		return Task{}, errors.New("cannot change human review requirement for a terminal task")
	}
	t.humanReview = true
	t.updatedAt = time.Now().UTC()
	return t, nil
}

func (t Task) DisableHumanReview() (Task, error) {
	if t.status == StatusReview || t.status == StatusHumanReview || t.status == StatusDone || t.status == StatusCancelled {
		return Task{}, errors.New("cannot disable human review after the task reaches review")
	}
	t.humanReview = false
	t.updatedAt = time.Now().UTC()
	return t, nil
}

func (t Task) Transition(to Status) (Task, error) {
	if !validStatus(to) {
		return Task{}, fmt.Errorf("unknown task status %q", to)
	}
	if t.humanReview && t.status == StatusReview && to == StatusDone {
		return Task{}, fmt.Errorf("%w: human review is required before completion", ErrInvalidTransition)
	}
	if !allowedTransition(t.status, to) {
		return Task{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, t.status, to)
	}
	t.status = to
	t.updatedAt = time.Now().UTC()
	return t, nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusBacklog, StatusReady, StatusWorking, StatusReview, StatusHumanReview,
		StatusBlocked, StatusFailed, StatusDone, StatusCancelled:
		return true
	default:
		return false
	}
}

func allowedTransition(from, to Status) bool {
	switch from {
	case StatusBacklog:
		return to == StatusReady || to == StatusCancelled
	case StatusReady:
		return to == StatusBacklog || to == StatusWorking || to == StatusCancelled
	case StatusWorking:
		return to == StatusReview || to == StatusBlocked || to == StatusFailed || to == StatusCancelled
	case StatusReview:
		return to == StatusWorking || to == StatusHumanReview || to == StatusDone || to == StatusFailed || to == StatusCancelled
	case StatusHumanReview:
		return to == StatusWorking || to == StatusDone || to == StatusCancelled
	case StatusBlocked:
		return to == StatusReady || to == StatusWorking || to == StatusFailed || to == StatusCancelled
	case StatusFailed:
		return to == StatusReady || to == StatusCancelled
	default:
		return false
	}
}
