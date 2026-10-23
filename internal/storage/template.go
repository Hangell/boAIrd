package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"boAIrd/internal/agent"
	"boAIrd/internal/template"
)

// TemplateRepository persists reusable agent templates and their instances.
type TemplateRepository struct{ database *Database }

func NewTemplateRepository(database *Database) (*TemplateRepository, error) {
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
// records the audit event atomically. expectedVersion is a compare-and-swap
// guard: the update is applied only if the stored version still matches, so
// concurrent updates cannot silently overwrite each other.
func (r *TemplateRepository) UpdateWithAudit(ctx context.Context, value template.Template, expectedVersion int, event template.AuditEvent) error {
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
		WHERE template_key = ? AND version = ?`,
		value.Name, value.Role, value.Description, value.Instructions, value.DefaultRuntime,
		tools, capabilities, value.Version, value.UpdatedBy, formatDatabaseTime(value.UpdatedAt), value.Key, expectedVersion)
	if err != nil {
		return fmt.Errorf("update agent template: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated agent templates: %w", err)
	}
	if count == 0 {
		return template.ErrVersionConflict
	}
	if err := insertTemplateAudit(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent template update: %w", err)
	}
	return nil
}

// InstantiateAgent creates the Agent, its project membership, its template
// provenance, and both audit events in one SQLite transaction so a partial
// failure never leaves an Agent without scope, provenance, or audit.
func (r *TemplateRepository) InstantiateAgent(ctx context.Context, value agent.Agent, instance template.Instance, event template.AuditEvent) (agent.Agent, error) {
	if event.ProjectID != instance.ProjectID || event.TemplateKey != instance.TemplateKey || event.ActorID == "" {
		return agent.Agent{}, errors.New("template instance audit metadata does not match instance")
	}
	snapshot := value.Snapshot()
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return agent.Agent{}, fmt.Errorf("begin template instantiation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockActiveProjectForInstantiation(ctx, tx, instance.ProjectID); err != nil {
		return agent.Agent{}, err
	}
	if err := validateAgentCredential(ctx, tx, snapshot); err != nil {
		return agent.Agent{}, err
	}
	if err := insertAgent(ctx, tx, snapshot); err != nil {
		return agent.Agent{}, fmt.Errorf("insert instantiated agent: %w", err)
	}
	if err := insertAgentRelations(ctx, tx, snapshot); err != nil {
		return agent.Agent{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_projects(agent_id, project_id) VALUES (?, ?)`,
		snapshot.ID, instance.ProjectID); err != nil {
		return agent.Agent{}, fmt.Errorf("insert instantiated agent project membership: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, result, reason)
		VALUES (?, 'administrator', ?, 'agent.project.joined', 'agent', ?, 'success', 'project scope granted')`,
		instance.ProjectID, event.ActorID, snapshot.ID); err != nil {
		return agent.Agent{}, fmt.Errorf("record instantiated agent membership audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO template_instances(agent_id, project_id, template_key, template_version, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		instance.AgentID, instance.ProjectID, instance.TemplateKey, instance.TemplateVersion, formatDatabaseTime(instance.CreatedAt)); err != nil {
		return agent.Agent{}, fmt.Errorf("insert template instance: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, result, reason)
		VALUES (?, 'administrator', ?, 'template.instantiated', 'agent', ?, 'success', ?)`,
		instance.ProjectID, event.ActorID, instance.AgentID, "agent instantiated from template "+instance.TemplateKey); err != nil {
		return agent.Agent{}, fmt.Errorf("record template instantiation audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return agent.Agent{}, fmt.Errorf("commit template instantiation: %w", err)
	}
	return reloadAgent(ctx, r.database, snapshot.ID)
}

// lockActiveProjectForInstantiation takes SQLite's write lock before reading
// project state, so archival either commits first and is rejected or waits
// until the complete instantiation transaction commits.
func lockActiveProjectForInstantiation(ctx context.Context, tx *Transaction, projectID string) error {
	result, err := tx.ExecContext(ctx, `UPDATE projects SET updated_at = updated_at WHERE id = ? AND status <> 'ARCHIVED'`, projectID)
	if err != nil {
		return fmt.Errorf("lock active project for template instantiation: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count active project lock rows: %w", err)
	}
	if updated > 0 {
		return nil
	}
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM projects WHERE id = ?`, projectID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return template.ErrProjectNotFound
		}
		return fmt.Errorf("read project status for template instantiation: %w", err)
	}
	if status == "ARCHIVED" {
		return template.ErrProjectArchived
	}
	return errors.New("lock project for template instantiation: no active project row matched")
}

// reloadAgent loads an Agent with its relations after an atomic write.
func reloadAgent(ctx context.Context, database *Database, id string) (agent.Agent, error) {
	snapshot, err := scanAgent(database.QueryRowContext(ctx, agentSelect+" WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return agent.Agent{}, agent.ErrNotFound
	}
	if err != nil {
		return agent.Agent{}, fmt.Errorf("read instantiated agent: %w", err)
	}
	if err := loadAgentRelations(ctx, database, &snapshot); err != nil {
		return agent.Agent{}, err
	}
	value, err := agent.Restore(snapshot)
	if err != nil {
		return agent.Agent{}, fmt.Errorf("restore instantiated agent: %w", err)
	}
	return value, nil
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
func insertTemplateAudit(ctx context.Context, tx *Transaction, event template.AuditEvent) error {
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
