package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"boAIrd/internal/template"
)

// TemplateRepository persists reusable agent templates and their instances.
type TemplateRepository struct{ database *sql.DB }

func NewTemplateRepository(database *sql.DB) (*TemplateRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &TemplateRepository{database: database}, nil
}

func (r *TemplateRepository) Get(ctx context.Context, key string) (template.Template, error) {
	row := r.database.QueryRowContext(ctx, `SELECT id, template_key, name, role, description, instructions,
		default_runtime, suggested_tools, suggested_capabilities, version, created_by, updated_by, created_at, updated_at
		FROM agent_templates WHERE template_key = ?`, key)
	value, err := scanTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return template.Template{}, template.ErrNotFound
	}
	if err != nil {
		return template.Template{}, err
	}
	return value, nil
}

func (r *TemplateRepository) List(ctx context.Context) ([]template.Template, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, template_key, name, role, description, instructions,
		default_runtime, suggested_tools, suggested_capabilities, version, created_by, updated_by, created_at, updated_at
		FROM agent_templates ORDER BY template_key`)
	if err != nil {
		return nil, fmt.Errorf("query agent templates: %w", err)
	}
	values := make([]template.Template, 0)
	for rows.Next() {
		value, err := scanTemplate(rows)
		if err != nil {
			return nil, closeRowsAfter(rows, "agent templates", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "agent templates", fmt.Errorf("iterate agent templates: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close agent template rows: %w", err)
	}
	return values, nil
}

// CreateWithAudit inserts a new template and its audit event atomically.
func (r *TemplateRepository) CreateWithAudit(ctx context.Context, value template.Template, event template.AuditEvent) error {
	tools, capabilities, err := marshalTemplateLists(value)
	if err != nil {
		return err
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin template creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_templates(
		id, template_key, name, role, description, instructions, default_runtime, suggested_tools, suggested_capabilities, version, created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.Key, value.Name, value.Role, value.Description, value.Instructions, value.DefaultRuntime,
		tools, capabilities, value.Version, value.CreatedBy, value.UpdatedBy, formatDatabaseTime(value.CreatedAt), formatDatabaseTime(value.UpdatedAt)); err != nil {
		return fmt.Errorf("insert agent template: %w", err)
	}
	if err := insertTemplateAudit(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent template creation: %w", err)
	}
	return nil
}

// UpdateWithAudit replaces a template's mutable fields, bumps its version, and
// records the audit event atomically.
func (r *TemplateRepository) UpdateWithAudit(ctx context.Context, value template.Template, event template.AuditEvent) error {
	tools, capabilities, err := marshalTemplateLists(value)
	if err != nil {
		return err
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin template update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE agent_templates SET name = ?, role = ?, description = ?, instructions = ?,
		default_runtime = ?, suggested_tools = ?, suggested_capabilities = ?, version = ?, updated_by = ?, updated_at = ?
		WHERE template_key = ?`,
		value.Name, value.Role, value.Description, value.Instructions, value.DefaultRuntime,
		tools, capabilities, value.Version, value.UpdatedBy, formatDatabaseTime(value.UpdatedAt), value.Key)
	if err != nil {
		return fmt.Errorf("update agent template: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("count updated agent templates: %w", err)
	} else if count == 0 {
		return template.ErrNotFound
	}
	if err := insertTemplateAudit(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent template update: %w", err)
	}
	return nil
}

// RecordInstanceWithAudit persists the template source/version for an Agent and
// the instantiation audit event atomically.
func (r *TemplateRepository) RecordInstanceWithAudit(ctx context.Context, instance template.Instance, event template.AuditEvent) error {
	if event.ProjectID != instance.ProjectID || event.TemplateKey != instance.TemplateKey || event.ActorID == "" {
		return errors.New("template instance audit metadata does not match instance")
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin template instantiation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO template_instances(agent_id, project_id, template_key, template_version, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		instance.AgentID, instance.ProjectID, instance.TemplateKey, instance.TemplateVersion, formatDatabaseTime(instance.CreatedAt)); err != nil {
		return fmt.Errorf("insert template instance: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, reason)
		VALUES (?, 'administrator', ?, 'template.instantiated', 'agent', ?, ?)`,
		instance.ProjectID, event.ActorID, instance.AgentID, "agent instantiated from template "+instance.TemplateKey); err != nil {
		return fmt.Errorf("record template instantiation audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit template instantiation: %w", err)
	}
	return nil
}

func marshalTemplateLists(value template.Template) (string, string, error) {
	tools, err := json.Marshal(value.SuggestedTools)
	if err != nil {
		return "", "", fmt.Errorf("encode template suggested tools: %w", err)
	}
	capabilities, err := json.Marshal(value.SuggestedCapabilities)
	if err != nil {
		return "", "", fmt.Errorf("encode template suggested capabilities: %w", err)
	}
	return string(tools), string(capabilities), nil
}

// insertTemplateAudit records a global template create/update in the dedicated
// immutable template_audit stream.
func insertTemplateAudit(ctx context.Context, tx *sql.Tx, event template.AuditEvent) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO template_audit(template_key, action, version, actor_id)
		VALUES (?, ?, ?, ?)`,
		event.TemplateKey, event.Action, event.Version, event.ActorID); err != nil {
		return fmt.Errorf("record template audit: %w", err)
	}
	return nil
}

type templateScanner interface {
	Scan(dest ...any) error
}

func scanTemplate(row templateScanner) (template.Template, error) {
	var value template.Template
	var tools, capabilities, createdAt, updatedAt string
	if err := row.Scan(&value.ID, &value.Key, &value.Name, &value.Role, &value.Description, &value.Instructions,
		&value.DefaultRuntime, &tools, &capabilities, &value.Version, &value.CreatedBy, &value.UpdatedBy, &createdAt, &updatedAt); err != nil {
		return template.Template{}, fmt.Errorf("scan agent template: %w", err)
	}
	if err := json.Unmarshal([]byte(tools), &value.SuggestedTools); err != nil {
		return template.Template{}, fmt.Errorf("decode template suggested tools: %w", err)
	}
	if err := json.Unmarshal([]byte(capabilities), &value.SuggestedCapabilities); err != nil {
		return template.Template{}, fmt.Errorf("decode template suggested capabilities: %w", err)
	}
	var err error
	if value.CreatedAt, err = parseDatabaseTime(createdAt); err != nil {
		return template.Template{}, fmt.Errorf("parse template creation time: %w", err)
	}
	if value.UpdatedAt, err = parseDatabaseTime(updatedAt); err != nil {
		return template.Template{}, fmt.Errorf("parse template update time: %w", err)
	}
	return value, nil
}
