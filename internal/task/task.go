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
	ID          string
	ProjectID   string
	Title       string
	Description string
	Status      Status
	Priority    string
	CreatedBy   string
	AssignedTo  string
	HumanReview bool
	ParentID    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func New(id, projectID, title, description, createdBy string) (Task, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(projectID) == "" ||
		strings.TrimSpace(title) == "" || strings.TrimSpace(createdBy) == "" {
		return Task{}, errors.New("task ID, project ID, title, and creator ID are required")
	}
	now := time.Now().UTC()
	return Task{
		ID: id, ProjectID: projectID, Title: title, Description: description,
		Status: StatusBacklog, CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (t Task) SetReferences(assignedTo, parentID string) (Task, error) {
	assignedTo = strings.TrimSpace(assignedTo)
	parentID = strings.TrimSpace(parentID)
	if assignedTo == t.ID || (parentID != "" && parentID == t.ID) {
		return Task{}, errors.New("task cannot reference itself")
	}
	t.AssignedTo = assignedTo
	t.ParentID = parentID
	t.UpdatedAt = time.Now().UTC()
	return t, nil
}

func (t Task) Transition(to Status) (Task, error) {
	if !validStatus(to) {
		return Task{}, fmt.Errorf("unknown task status %q", to)
	}
	if t.HumanReview && t.Status == StatusReview && to == StatusDone {
		return Task{}, fmt.Errorf("%w: human review is required before completion", ErrInvalidTransition)
	}
	if !allowedTransition(t.Status, to) {
		return Task{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, t.Status, to)
	}
	t.Status = to
	t.UpdatedAt = time.Now().UTC()
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
