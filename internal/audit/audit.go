// Package audit defines immutable operational and security audit records.
package audit

import (
	"context"
	"time"
)

// Escalation records that orchestration could not resolve a task without administrator action.
type Escalation struct {
	ProjectID string
	TaskID    string
	RunID     string
	AgentID   string
	Attempts  int
	Status    string
	Reason    string
}

// Recorder persists administrator escalations.
type Recorder interface {
	RecordEscalation(context.Context, Escalation) error
}

// Event is one append-only security audit record. Action is the event name and
// Result captures the outcome (for example "success" or "denied").
type Event struct {
	ProjectID    string
	TaskID       string
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Result       string
	Reason       string
}

// EventRecorder appends security audit events.
type EventRecorder interface {
	RecordEvent(context.Context, Event) error
}

// Entry is an audit event read back from storage.
type Entry struct {
	ID           string
	ProjectID    string
	TaskID       string
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Result       string
	Reason       string
	CreatedAt    time.Time
}

// Reader lists project-scoped audit events, newest first.
type Reader interface {
	ListEvents(context.Context, string, int, int) ([]Entry, error)
}
