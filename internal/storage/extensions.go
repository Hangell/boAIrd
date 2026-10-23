package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"boAIrd/internal/extension"
)

// ExtensionRepository persists extension manifests and lifecycle state.
type ExtensionRepository struct {
	database *Database
}

func NewExtensionRepository(database *Database) (*ExtensionRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &ExtensionRepository{database: database}, nil
}

// InstallWithAudit stores a manifest, reserves its tool names, and appends the
// installation event in one transaction.
func (r *ExtensionRepository) InstallWithAudit(ctx context.Context, value extension.Extension, actorID string) error {
	if err := validateExtensionAuditInput(actorID, value.Name(), "extension.installed"); err != nil {
		return err
	}
	manifest, err := json.Marshal(value.Tools())
	if err != nil {
		return fmt.Errorf("marshal extension manifest: %w", err)
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin extension installation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO extensions(name, version, manifest, status) VALUES (?, ?, ?, ?)`,
		value.Name(), value.Version(), string(manifest), string(value.Status())); err != nil {
		if isUniqueViolation(err) {
			return extension.ErrAlreadyExists
		}
		return fmt.Errorf("insert extension: %w", err)
	}
	for _, tool := range value.Tools() {
		if _, err := tx.ExecContext(ctx, `INSERT INTO extension_tool_names(tool_name, extension_name) VALUES (?, ?)`, tool.Name, value.Name()); err != nil {
			if isUniqueViolation(err) {
				return extension.ErrToolNameConflict
			}
			return fmt.Errorf("reserve extension tool name: %w", err)
		}
	}
	if err := recordExtensionEventTx(ctx, tx, actorID, value.Name(), "extension.installed"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit extension installation: %w", err)
	}
	return nil
}

// ActivateWithAudit uses a status compare-and-set and writes the audit event atomically.
func (r *ExtensionRepository) ActivateWithAudit(ctx context.Context, name, actorID string) error {
	return r.setStatusWithAudit(ctx, name, actorID, extension.StatusPending, extension.StatusActive, "extension.activated")
}

func (r *ExtensionRepository) setStatusWithAudit(ctx context.Context, name, actorID string, from, to extension.Status, eventType string) error {
	if err := validateExtensionAuditInput(actorID, name, eventType); err != nil {
		return err
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin extension status update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE extensions SET status = ? WHERE name = ? AND status = ?`, string(to), name, string(from))
	if err != nil {
		return fmt.Errorf("update extension status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated extension: %w", err)
	}
	if affected == 0 {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM extensions WHERE name = ?`, name).Scan(&status); errors.Is(err, sql.ErrNoRows) {
			return extension.ErrNotFound
		} else if err != nil {
			return fmt.Errorf("inspect extension status: %w", err)
		}
		return extension.ErrWrongStatus
	}
	if err := recordExtensionEventTx(ctx, tx, actorID, name, eventType); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit extension status update: %w", err)
	}
	return nil
}

// DeactivateWithAudit removes active state, reservations, and its event atomically.
func (r *ExtensionRepository) DeactivateWithAudit(ctx context.Context, name, actorID string) error {
	if err := validateExtensionAuditInput(actorID, name, "extension.deactivated"); err != nil {
		return err
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin extension deactivation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM extensions WHERE name = ? AND status = ?`, name, string(extension.StatusActive))
	if err != nil {
		return fmt.Errorf("delete extension: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted extension: %w", err)
	}
	if affected == 0 {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM extensions WHERE name = ?`, name).Scan(&status); errors.Is(err, sql.ErrNoRows) {
			return extension.ErrNotFound
		} else if err != nil {
			return fmt.Errorf("inspect extension before delete: %w", err)
		}
		return extension.ErrWrongStatus
	}
	if err := recordExtensionEventTx(ctx, tx, actorID, name, "extension.deactivated"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit extension deactivation: %w", err)
	}
	return nil
}

func (r *ExtensionRepository) List(ctx context.Context) ([]extension.Extension, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT name, version, manifest, status FROM extensions ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list extensions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	values := make([]extension.Extension, 0)
	for rows.Next() {
		value, err := scanExtension(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate extensions: %w", err)
	}
	return values, nil
}

func (r *ExtensionRepository) Get(ctx context.Context, name string) (extension.Extension, error) {
	value, err := scanExtension(r.database.QueryRowContext(ctx, `SELECT name, version, manifest, status FROM extensions WHERE name = ?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return extension.Extension{}, extension.ErrNotFound
	}
	if err != nil {
		return extension.Extension{}, fmt.Errorf("get extension: %w", err)
	}
	return value, nil
}

// IsActiveTool resolves persisted lifecycle state before an extension adapter
// runs, so an in-memory registration cannot outlive deactivation.
func (r *ExtensionRepository) IsActiveTool(ctx context.Context, toolName string) (bool, error) {
	var active bool
	if err := r.database.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM extension_tool_names AS names
		JOIN extensions ON extensions.name = names.extension_name
		WHERE names.tool_name = ? AND extensions.status = ?
	)`, toolName, string(extension.StatusActive)).Scan(&active); err != nil {
		return false, fmt.Errorf("check active extension tool: %w", err)
	}
	return active, nil
}

// RecordExtensionEvent appends an immutable extension lifecycle event.
func (r *ExtensionRepository) RecordExtensionEvent(ctx context.Context, actorID, extensionName, eventType string) error {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(extensionName) == "" || strings.TrimSpace(eventType) == "" {
		return errors.New("extension audit actor, extension, and event are required")
	}
	if _, err := r.database.ExecContext(ctx, `INSERT INTO extension_audit(actor_id, extension_name, event_type) VALUES (?, ?, ?)`,
		actorID, extensionName, eventType); err != nil {
		return fmt.Errorf("record extension event: %w", err)
	}
	return nil
}

func validateExtensionAuditInput(actorID, extensionName, eventType string) error {
	if strings.TrimSpace(actorID) == "" || len(actorID) > 200 || strings.TrimSpace(extensionName) == "" || len(extensionName) > 64 || strings.TrimSpace(eventType) == "" || len(eventType) > 100 {
		return errors.New("extension audit actor, extension, and event are required and bounded")
	}
	return nil
}

func recordExtensionEventTx(ctx context.Context, tx *Transaction, actorID, extensionName, eventType string) error {
	if err := validateExtensionAuditInput(actorID, extensionName, eventType); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO extension_audit(actor_id, extension_name, event_type) VALUES (?, ?, ?)`, actorID, extensionName, eventType); err != nil {
		return fmt.Errorf("record extension event: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "unique constraint") || strings.Contains(value, "duplicate key") || strings.Contains(value, "23505")
}

func scanExtension(row rowScanner) (extension.Extension, error) {
	var name, version, manifest, status string
	if err := row.Scan(&name, &version, &manifest, &status); err != nil {
		return extension.Extension{}, err
	}
	var tools []extension.ToolDeclaration
	if err := json.Unmarshal([]byte(manifest), &tools); err != nil {
		return extension.Extension{}, fmt.Errorf("unmarshal extension manifest: %w", err)
	}
	return extension.Restore(name, version, extension.Status(status), tools)
}
