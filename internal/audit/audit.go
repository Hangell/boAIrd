// Package audit defines immutable operational and security audit records.
package audit

import "context"

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
