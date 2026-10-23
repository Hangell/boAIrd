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
	if _, err := r.database.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, result, reason)
		VALUES (?, ?, ?, 'tool.executed', 'tool', ?, ?, ?)`,
		event.ProjectID, event.ActorType, event.ActorID, event.ToolName, event.Outcome, reason); err != nil {
		return fmt.Errorf("record tool execution: %w", err)
	}
	return nil
}

// RecordEvent appends one security audit event. Actor, action, resource, and
// result are required; the reason is redacted and bounded before persistence.
func (r *AuditRepository) RecordEvent(ctx context.Context, event audit.Event) error {
	if strings.TrimSpace(event.ProjectID) == "" || strings.TrimSpace(event.ActorType) == "" ||
		strings.TrimSpace(event.ActorID) == "" || strings.TrimSpace(event.Action) == "" ||
		strings.TrimSpace(event.ResourceType) == "" || strings.TrimSpace(event.ResourceID) == "" {
		return errors.New("audit event project, actor, action, and resource are required")
	}
	reason := logging.RedactSensitiveText(strings.TrimSpace(event.Reason))
	if len(reason) > 2000 {
		reason = reason[:2000]
	}
	if _, err := r.database.ExecContext(ctx, `INSERT INTO audit_events(project_id, task_id, actor_type, actor_id, event_type, resource_type, resource_id, result, reason)
		VALUES (?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)`,
		event.ProjectID, event.TaskID, event.ActorType, event.ActorID, event.Action,
		event.ResourceType, event.ResourceID, event.Result, reason); err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

// ListEvents returns project-scoped audit events, newest first, with reason
// redacted on read as defense in depth.
func (r *AuditRepository) ListEvents(ctx context.Context, projectID string, limit, offset int) ([]audit.Entry, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, project_id, COALESCE(task_id, ''), actor_type, actor_id,
		event_type, resource_type, resource_id, COALESCE(result, ''), COALESCE(reason, ''), created_at
		FROM audit_events WHERE project_id = ? ORDER BY sequence DESC LIMIT ? OFFSET ?`, projectID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query audit events: %w", err)
	}
	events := make([]audit.Entry, 0)
	for rows.Next() {
		var entry audit.Entry
		var created string
		if err := rows.Scan(&entry.ID, &entry.ProjectID, &entry.TaskID, &entry.ActorType, &entry.ActorID,
			&entry.Action, &entry.ResourceType, &entry.ResourceID, &entry.Result, &entry.Reason, &created); err != nil {
			return nil, closeRowsAfter(rows, "audit events", fmt.Errorf("scan audit event: %w", err))
		}
		entry.Reason = logging.RedactSensitiveText(entry.Reason)
		entry.CreatedAt, err = parseDatabaseTime(created)
		if err != nil {
			return nil, closeRowsAfter(rows, "audit events", fmt.Errorf("parse audit event time: %w", err))
		}
		events = append(events, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "audit events", fmt.Errorf("iterate audit events: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close audit event rows: %w", err)
	}
	return events, nil
}
