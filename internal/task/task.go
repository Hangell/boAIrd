// Package task defines provider-independent work items and their lifecycle.
package task

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type Status string
type ActorType string

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

const (
	ActorLocalAPI      ActorType = "LOCAL_API_CLIENT"
	ActorAdministrator ActorType = "ADMINISTRATOR"
	ActorBrain         ActorType = "BRAIN"
	ActorAgent         ActorType = "AGENT"
)

var ErrInvalidTransition = errors.New("invalid task status transition")

type Task struct {
	id            string
	projectID     string
	title         string
	description   string
	status        Status
	priority      string
	createdBy     string
	createdByType ActorType
	assignedTo    string
	humanReview   bool
	parentID      string
	createdAt     time.Time
	updatedAt     time.Time
	lastActorType ActorType
	lastActorID   string
	lastReason    string
}

type Actor struct {
	Type         ActorType
	ID           string
	Capabilities []string
	ProjectIDs   []string
}

type Snapshot struct {
	ID            string
	ProjectID     string
	Title         string
	Description   string
	Status        Status
	Priority      string
	CreatedBy     string
	CreatedByType ActorType
	AssignedTo    string
	HumanReview   bool
	ParentID      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastActorType ActorType
	LastActorID   string
	LastReason    string
}

func New(id, projectID, title, description, createdBy string) (Task, error) {
	return NewWithActor(id, projectID, title, description, Actor{Type: ActorAgent, ID: createdBy})
}

func NewWithActor(id, projectID, title, description string, actor Actor) (Task, error) {
	id, projectID, title, description, actor.ID = strings.TrimSpace(id), strings.TrimSpace(projectID), strings.TrimSpace(title), strings.TrimSpace(description), strings.TrimSpace(actor.ID)
	if strings.TrimSpace(id) == "" || strings.TrimSpace(projectID) == "" ||
		strings.TrimSpace(title) == "" || strings.TrimSpace(actor.ID) == "" || !validActorType(actor.Type) {
		return Task{}, errors.New("task ID, project ID, title, and valid creator are required")
	}
	if utf8.RuneCountInString(title) > 200 || len(description) > 16000 || strings.ContainsRune(title+description, '\x00') {
		return Task{}, errors.New("task title or description exceeds its allowed size")
	}
	now := time.Now().UTC()
	return Task{
		id: id, projectID: projectID, title: title, description: description,
		status: StatusBacklog, priority: "NORMAL", createdBy: actor.ID, createdByType: actor.Type, lastActorType: actor.Type,
		lastActorID: actor.ID, createdAt: now, updatedAt: now,
	}, nil
}

func (t Task) ID() string               { return t.id }
func (t Task) ProjectID() string        { return t.projectID }
func (t Task) Title() string            { return t.title }
func (t Task) Description() string      { return t.description }
func (t Task) Status() Status           { return t.status }
func (t Task) Priority() string         { return t.priority }
func (t Task) CreatedBy() string        { return t.createdBy }
func (t Task) CreatedByType() ActorType { return t.createdByType }
func (t Task) AssignedTo() string       { return t.assignedTo }
func (t Task) HumanReview() bool        { return t.humanReview }
func (t Task) ParentID() string         { return t.parentID }
func (t Task) CreatedAt() time.Time     { return t.createdAt }
func (t Task) UpdatedAt() time.Time     { return t.updatedAt }
func (t Task) LastActorType() ActorType { return t.lastActorType }
func (t Task) LastActorID() string      { return t.lastActorID }
func (t Task) LastReason() string       { return t.lastReason }

func (t Task) Snapshot() Snapshot {
	return Snapshot{ID: t.id, ProjectID: t.projectID, Title: t.title, Description: t.description, Status: t.status, Priority: t.priority,
		CreatedBy: t.createdBy, CreatedByType: t.createdByType, AssignedTo: t.assignedTo, HumanReview: t.humanReview, ParentID: t.parentID,
		CreatedAt: t.createdAt, UpdatedAt: t.updatedAt, LastActorType: t.lastActorType, LastActorID: t.lastActorID, LastReason: t.lastReason}
}

func Restore(snapshot Snapshot) (Task, error) {
	if strings.TrimSpace(snapshot.ID) == "" || strings.TrimSpace(snapshot.ProjectID) == "" || strings.TrimSpace(snapshot.Title) == "" ||
		strings.TrimSpace(snapshot.CreatedBy) == "" || !validActorType(snapshot.CreatedByType) || !validStatus(snapshot.Status) ||
		!validActorType(snapshot.LastActorType) || strings.TrimSpace(snapshot.LastActorID) == "" || snapshot.CreatedAt.IsZero() || snapshot.UpdatedAt.IsZero() {
		return Task{}, errors.New("invalid task snapshot")
	}
	return Task{id: snapshot.ID, projectID: snapshot.ProjectID, title: snapshot.Title, description: snapshot.Description, status: snapshot.Status,
		priority: snapshot.Priority, createdBy: snapshot.CreatedBy, createdByType: snapshot.CreatedByType, assignedTo: snapshot.AssignedTo,
		humanReview: snapshot.HumanReview, parentID: snapshot.ParentID, createdAt: snapshot.CreatedAt.UTC(), updatedAt: snapshot.UpdatedAt.UTC(),
		lastActorType: snapshot.LastActorType, lastActorID: snapshot.LastActorID, lastReason: snapshot.LastReason}, nil
}

func (t Task) SetPriority(priority string) (Task, error) {
	priority = strings.ToUpper(strings.TrimSpace(priority))
	if priority == "" {
		priority = "NORMAL"
	}
	switch priority {
	case "LOW", "NORMAL", "HIGH", "URGENT":
	default:
		return Task{}, errors.New("invalid task priority")
	}
	t.priority = priority
	t.updatedAt = time.Now().UTC()
	return t, nil
}

func (t Task) RecordMutation(actor Actor, reason string) (Task, error) {
	actor.ID = strings.TrimSpace(actor.ID)
	reason = strings.TrimSpace(reason)
	if !validActorType(actor.Type) || actor.ID == "" || reason == "" || len(reason) > 2000 {
		return Task{}, errors.New("valid actor and reason are required")
	}
	t.lastActorType, t.lastActorID, t.lastReason = actor.Type, actor.ID, reason
	t.updatedAt = time.Now().UTC()
	return t, nil
}

func (t Task) Assign(agentID string) (Task, error) {
	if terminalStatus(t.status) {
		return Task{}, ErrInvalidTransition
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return Task{}, errors.New("agent ID is required")
	}
	t.assignedTo = agentID
	t.updatedAt = time.Now().UTC()
	return t, nil
}

func (t Task) Unassign() (Task, error) {
	if terminalStatus(t.status) {
		return Task{}, ErrInvalidTransition
	}
	t.assignedTo = ""
	t.updatedAt = time.Now().UTC()
	return t, nil
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
	if terminalStatus(t.status) {
		return Task{}, ErrInvalidTransition
	}
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

func terminalStatus(status Status) bool {
	return status == StatusDone || status == StatusCancelled
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

func validActorType(value ActorType) bool {
	switch value {
	case ActorLocalAPI, ActorAdministrator, ActorBrain, ActorAgent:
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
