package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"boAIrd/internal/audit"
	"boAIrd/internal/logging"
	"boAIrd/internal/tool"
)

// AuditRepository appends immutable audit events that are not produced by domain triggers.
type AuditRepository struct {
	database *sql.DB
}

func NewAuditRepository(database *sql.DB) (*AuditRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &AuditRepository{database: database}, nil
}

func (r *AuditRepository) RecordEscalation(ctx context.Context, escalation audit.Escalation) error {
	if strings.TrimSpace(escalation.ProjectID) == "" || strings.TrimSpace(escalation.TaskID) == "" {
		return errors.New("escalation project and task are required")
	}
	reason := logging.RedactSensitiveText(strings.TrimSpace(escalation.Reason))
	if len(reason) > 2000 {
		reason = reason[:2000]
	}
	if _, err := r.database.ExecContext(ctx, `INSERT INTO audit_events(project_id, task_id, actor_type, actor_id, event_type, resource_type, resource_id, to_status, reason)
		VALUES (?, ?, 'system', 'orchestrator', 'orchestrator.escalated', 'task', ?, ?, ?)`,
		escalation.ProjectID, escalation.TaskID, escalation.TaskID, escalation.Status, reason); err != nil {
		return fmt.Errorf("record orchestration escalation: %w", err)
	}
	return nil
}

// RecordToolExecution appends metadata-only tool activity to immutable audit history.
func (r *AuditRepository) RecordToolExecution(ctx context.Context, event tool.Event) error {
	if strings.TrimSpace(event.ProjectID) == "" || strings.TrimSpace(event.ActorType) == "" ||
		strings.TrimSpace(event.ActorID) == "" || strings.TrimSpace(event.ToolName) == "" ||
		strings.TrimSpace(event.Outcome) == "" {
		return errors.New("tool audit project, actor, tool, and outcome are required")
	}
	reason := "outcome=" + event.Outcome
	if event.ResourceType != "" && event.ResourceID != "" {
		reason += "; resource=" + event.ResourceType + ":" + event.ResourceID
	}
	reason = logging.RedactSensitiveText(reason)
	if len(reason) > 2000 {
		reason = reason[:2000]
	}
	if _, err := r.database.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, to_status, reason)
		VALUES (?, ?, ?, 'tool.executed', 'tool', ?, ?, ?)`,
		event.ProjectID, event.ActorType, event.ActorID, event.ToolName, event.Outcome, reason); err != nil {
		return fmt.Errorf("record tool execution: %w", err)
	}
	return nil
}
