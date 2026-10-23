package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"boAIrd/internal/audit"
	"boAIrd/internal/logging"
	"boAIrd/internal/profile"
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

// RecordProfileChange appends an immutable audit event for a profile override change.
func (r *AuditRepository) RecordProfileChange(ctx context.Context, event profile.AuditEvent) error {
	if strings.TrimSpace(event.ProjectID) == "" || strings.TrimSpace(event.ProfileKey) == "" {
		return errors.New("profile change project and profile key are required")
	}
	eventType := "profile.created"
	if event.Action == "updated" {
		eventType = "profile.updated"
	}
	if _, err := r.database.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, reason)
		VALUES (?, 'administrator', ?, ?, 'profile', ?, 'project profile configuration changed')`,
		event.ProjectID, event.ActorID, eventType, event.ProfileKey); err != nil {
		return fmt.Errorf("record profile change: %w", err)
	}
	return nil
}
