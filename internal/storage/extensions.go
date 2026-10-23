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

func (r *ExtensionRepository) Create(ctx context.Context, value extension.Extension) error {
	manifest, err := json.Marshal(value.Tools())
	if err != nil {
		return fmt.Errorf("marshal extension manifest: %w", err)
	}
	if _, err := r.database.ExecContext(ctx, `INSERT INTO extensions(name, version, manifest, status) VALUES (?, ?, ?, ?)`,
		value.Name(), value.Version(), string(manifest), string(value.Status())); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
			return extension.ErrAlreadyExists
		}
		return fmt.Errorf("insert extension: %w", err)
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

func (r *ExtensionRepository) SetStatus(ctx context.Context, name string, status extension.Status) error {
	result, err := r.database.ExecContext(ctx, `UPDATE extensions SET status = ? WHERE name = ?`, string(status), name)
	if err != nil {
		return fmt.Errorf("set extension status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated extension: %w", err)
	}
	if affected == 0 {
		return extension.ErrNotFound
	}
	return nil
}

func (r *ExtensionRepository) Delete(ctx context.Context, name string) error {
	if _, err := r.database.ExecContext(ctx, `DELETE FROM extensions WHERE name = ?`, name); err != nil {
		return fmt.Errorf("delete extension: %w", err)
	}
	return nil
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
