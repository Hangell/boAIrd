package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"boAIrd/internal/profile"
)

// ProfileRepository persists project-scoped profile overrides.
type ProfileRepository struct{ database *sql.DB }

func NewProfileRepository(database *sql.DB) (*ProfileRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &ProfileRepository{database: database}, nil
}

func (r *ProfileRepository) Get(ctx context.Context, projectID, key string) (profile.Override, error) {
	row := r.database.QueryRowContext(ctx, `SELECT id, project_id, profile_key, instructions, capabilities, tools,
		created_by, updated_by, created_at, updated_at
		FROM project_profiles WHERE project_id = ? AND profile_key = ?`, projectID, key)
	value, err := scanProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return profile.Override{}, profile.ErrNotFound
	}
	if err != nil {
		return profile.Override{}, err
	}
	return value, nil
}

func (r *ProfileRepository) List(ctx context.Context, projectID string) ([]profile.Override, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, project_id, profile_key, instructions, capabilities, tools,
		created_by, updated_by, created_at, updated_at
		FROM project_profiles WHERE project_id = ? ORDER BY profile_key`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query project profiles: %w", err)
	}
	values := make([]profile.Override, 0)
	for rows.Next() {
		value, err := scanProfile(rows)
		if err != nil {
			return nil, closeRowsAfter(rows, "project profiles", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "project profiles", fmt.Errorf("iterate project profiles: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close project profile rows: %w", err)
	}
	return values, nil
}

// SaveWithAudit persists the override and its immutable audit event in one
// transaction so neither change can be committed without the other.
func (r *ProfileRepository) SaveWithAudit(ctx context.Context, value profile.Override, event profile.AuditEvent) error {
	if event.ProjectID != value.ProjectID || event.ProfileKey != value.ProfileKey || event.ActorID != value.UpdatedBy {
		return errors.New("profile audit metadata does not match override")
	}
	capabilities, err := json.Marshal(value.Capabilities)
	if err != nil {
		return fmt.Errorf("encode profile capabilities: %w", err)
	}
	tools, err := json.Marshal(value.Tools)
	if err != nil {
		return fmt.Errorf("encode profile tools: %w", err)
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin profile change transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `INSERT INTO project_profiles(
		id, project_id, profile_key, instructions, capabilities, tools, created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, profile_key) DO NOTHING`,
		value.ID, value.ProjectID, value.ProfileKey, value.Instructions, string(capabilities), string(tools),
		value.CreatedBy, value.UpdatedBy, formatDatabaseTime(value.CreatedAt), formatDatabaseTime(value.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert project profile: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check project profile insert: %w", err)
	}
	action := "created"
	if inserted == 0 {
		action = "updated"
		result, err = tx.ExecContext(ctx, `UPDATE project_profiles SET
			instructions = ?,
			capabilities = ?, tools = ?, updated_by = ?, updated_at = ?
			WHERE project_id = ? AND profile_key = ?`,
			value.Instructions, string(capabilities), string(tools), value.UpdatedBy, formatDatabaseTime(value.UpdatedAt),
			value.ProjectID, value.ProfileKey)
		if err != nil {
			return fmt.Errorf("update project profile: %w", err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check project profile update: %w", err)
		}
		if updated != 1 {
			return errors.New("project profile disappeared during update")
		}
	}
	eventType := "profile.created"
	if action == "updated" {
		eventType = "profile.updated"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, reason)
		VALUES (?, 'administrator', ?, ?, 'profile', ?, 'project profile configuration changed')`,
		event.ProjectID, event.ActorID, eventType, event.ProfileKey); err != nil {
		return fmt.Errorf("record profile change: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit profile change: %w", err)
	}
	return nil
}

type profileScanner interface {
	Scan(dest ...any) error
}

func scanProfile(row profileScanner) (profile.Override, error) {
	var value profile.Override
	var capabilities, tools, createdAt, updatedAt string
	if err := row.Scan(&value.ID, &value.ProjectID, &value.ProfileKey, &value.Instructions, &capabilities,
		&tools, &value.CreatedBy, &value.UpdatedBy, &createdAt, &updatedAt); err != nil {
		return profile.Override{}, fmt.Errorf("scan project profile: %w", err)
	}
	if err := json.Unmarshal([]byte(capabilities), &value.Capabilities); err != nil {
		return profile.Override{}, fmt.Errorf("decode profile capabilities: %w", err)
	}
	if err := json.Unmarshal([]byte(tools), &value.Tools); err != nil {
		return profile.Override{}, fmt.Errorf("decode profile tools: %w", err)
	}
	var err error
	if value.CreatedAt, err = parseDatabaseTime(createdAt); err != nil {
		return profile.Override{}, fmt.Errorf("parse profile creation time: %w", err)
	}
	if value.UpdatedAt, err = parseDatabaseTime(updatedAt); err != nil {
		return profile.Override{}, fmt.Errorf("parse profile update time: %w", err)
	}
	return value, nil
}
