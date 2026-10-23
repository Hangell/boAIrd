package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"boAIrd/internal/agent"
	"boAIrd/internal/project"
)

type AgentRepository struct{ database *Database }

func closeRowsAfter(rows *sql.Rows, operation string, operationErr error) error {
	if closeErr := rows.Close(); closeErr != nil {
		return errors.Join(operationErr, fmt.Errorf("close %s rows: %w", operation, closeErr))
	}
	return operationErr
}

func NewAgentRepository(database *Database) (*AgentRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &AgentRepository{database: database}, nil
}

func (r *AgentRepository) Create(ctx context.Context, value agent.Agent) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create agent: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateAgentCredential(ctx, tx, value.Snapshot()); err != nil {
		return err
	}
	if err := insertAgent(ctx, tx, value.Snapshot()); err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	if err := insertAgentRelations(ctx, tx, value.Snapshot()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent create: %w", err)
	}
	return nil
}

func (r *AgentRepository) Get(ctx context.Context, id string) (agent.Agent, error) {
	snapshot, err := scanAgent(r.database.QueryRowContext(ctx, agentSelect+" WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return agent.Agent{}, agent.ErrNotFound
	}
	if err != nil {
		return agent.Agent{}, fmt.Errorf("read agent: %w", err)
	}
	if err := loadAgentRelations(ctx, r.database, &snapshot); err != nil {
		return agent.Agent{}, err
	}
	value, err := agent.Restore(snapshot)
	if err != nil {
		return agent.Agent{}, fmt.Errorf("restore agent: %w", err)
	}
	return value, nil
}

func (r *AgentRepository) List(ctx context.Context, limit, offset int) ([]agent.Agent, error) {
	return r.list(ctx, agentSelect+" ORDER BY lower(name), id LIMIT ? OFFSET ?", limit, offset)
}

// ListForUser applies current membership scope before pagination. Active Brain
// Agents are visible through their Brain-to-project relation as well.
func (r *AgentRepository) ListForUser(ctx context.Context, userID string, limit, offset int) ([]agent.Agent, error) {
	query := agentSelect + ` WHERE EXISTS (
		SELECT 1 FROM agent_projects ap
		JOIN project_members pm ON pm.project_id = ap.project_id
		WHERE ap.agent_id = agents.id AND pm.user_id = ?
	) OR EXISTS (
		SELECT 1 FROM brain_configuration bc
		JOIN brains b ON b.id = bc.brain_id AND b.active = 1
		JOIN projects p ON p.id = b.project_id AND p.brain_id = b.id
		JOIN project_members pm ON pm.project_id = p.id
		WHERE bc.agent_id = agents.id AND pm.user_id = ?
	) ORDER BY lower(agents.name), agents.id LIMIT ? OFFSET ?`
	return r.list(ctx, query, userID, userID, limit, offset)
}

// HasProjectAssociation permits a member to read an Agent scoped directly or
// through an active Brain configuration in one of the member's projects.
func (r *AgentRepository) HasProjectAssociation(ctx context.Context, agentID, userID string) (bool, error) {
	query := `SELECT EXISTS(
		SELECT 1 FROM agent_projects ap
		JOIN project_members pm ON pm.project_id = ap.project_id
		WHERE ap.agent_id = ? AND pm.user_id = ?
		UNION ALL
		SELECT 1 FROM brain_configuration bc
		JOIN brains b ON b.id = bc.brain_id AND b.active = 1
		JOIN projects p ON p.id = b.project_id AND p.brain_id = b.id
		JOIN project_members pm ON pm.project_id = p.id
		WHERE bc.agent_id = ? AND pm.user_id = ?
	)`
	var associated bool
	if err := r.database.QueryRowContext(ctx, query, agentID, userID, agentID, userID).Scan(&associated); err != nil {
		return false, fmt.Errorf("check agent project association: %w", err)
	}
	return associated, nil
}

func (r *AgentRepository) list(ctx context.Context, query string, args ...any) ([]agent.Agent, error) {
	rows, err := r.database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query agents: %w", err)
	}
	snapshots := make([]agent.Snapshot, 0)
	for rows.Next() {
		snapshot, err := scanAgent(rows)
		if err != nil {
			return nil, closeRowsAfter(rows, "agents", fmt.Errorf("scan agent: %w", err))
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "agents", fmt.Errorf("iterate agents: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close agent rows: %w", err)
	}
	values := make([]agent.Agent, 0, len(snapshots))
	for i := range snapshots {
		if err := loadAgentRelations(ctx, r.database, &snapshots[i]); err != nil {
			return nil, err
		}
		value, err := agent.Restore(snapshots[i])
		if err != nil {
			return nil, fmt.Errorf("restore agent: %w", err)
		}
		values = append(values, value)
	}
	return values, nil
}

func (r *AgentRepository) Update(ctx context.Context, value agent.Agent) error {
	snapshot := value.Snapshot()
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update agent: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var previousCredentialID sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT credential_id FROM agents WHERE id=?", snapshot.ID).Scan(&previousCredentialID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agent.ErrNotFound
		}
		return fmt.Errorf("read existing agent credential scope: %w", err)
	}
	if err := validateAgentCredential(ctx, tx, snapshot); err != nil {
		return err
	}
	// Status is server-derived runtime state: configuration updates must not
	// overwrite it, otherwise a concurrent reconcile would be reverted.
	result, err := tx.ExecContext(ctx, `UPDATE agents SET name = ?, avatar = ?, description = ?, role = ?, runtime = ?,
		provider = ?, provider_base_url = ?, model = ?, credential_id = NULLIF(?, ''), instructions = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, snapshot.Name, snapshot.Avatar, snapshot.Description, snapshot.Role, snapshot.Runtime,
		snapshot.Provider, snapshot.ProviderBaseURL, snapshot.Model, snapshot.CredentialID, snapshot.Instructions, snapshot.ID)
	if err != nil {
		return fmt.Errorf("update agent: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated agents: %w", err)
	}
	if count == 0 {
		return agent.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM agent_languages WHERE agent_id = ?", snapshot.ID); err != nil {
		return fmt.Errorf("replace agent languages: %w", err)
	}
	if err := syncAgentCapabilities(ctx, tx, snapshot); err != nil {
		return err
	}
	if err := insertAgentLanguages(ctx, tx, snapshot); err != nil {
		return err
	}
	if previousCredentialID.String != snapshot.CredentialID {
		if err := auditAgentCredentialScopeChange(ctx, tx, snapshot.ID, previousCredentialID.String, snapshot.CredentialID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent update: %w", err)
	}
	return nil
}

func auditAgentCredentialScopeChange(ctx context.Context, tx *Transaction, agentID, previousCredentialID, nextCredentialID string) error {
	rows, err := tx.QueryContext(ctx, `SELECT project_id FROM agent_projects WHERE agent_id=?
		UNION SELECT p.id FROM brain_configuration bc JOIN brains b ON b.id=bc.brain_id AND b.active=1
		JOIN projects p ON p.id=b.project_id AND p.brain_id=b.id WHERE bc.agent_id=? ORDER BY project_id`, agentID, agentID)
	if err != nil {
		return fmt.Errorf("query projects for credential scope audit: %w", err)
	}
	var projectIDs []string
	for rows.Next() {
		var projectID string
		if err := rows.Scan(&projectID); err != nil {
			return closeRowsAfter(rows, "credential scope audit projects", fmt.Errorf("scan credential scope audit project: %w", err))
		}
		projectIDs = append(projectIDs, projectID)
	}
	if err := rows.Err(); err != nil {
		return closeRowsAfter(rows, "credential scope audit projects", fmt.Errorf("iterate credential scope audit projects: %w", err))
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close credential scope audit projects: %w", err)
	}
	for _, projectID := range projectIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, from_status, to_status, reason)
			VALUES (?, 'administrator', ?, 'agent.credential.changed', 'agent', ?, ?, ?, 'provider credential scope changed')`,
			projectID, project.LocalAdministratorID, agentID, previousCredentialID, nextCredentialID); err != nil {
			return fmt.Errorf("audit credential scope change: %w", err)
		}
	}
	return nil
}

func (r *AgentRepository) AddProject(ctx context.Context, agentID, projectID, actorID string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent project membership: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `INSERT INTO agent_projects(agent_id, project_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, agentID, projectID)
	if err != nil {
		return fmt.Errorf("add agent project membership: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count added agent project membership: %w", err)
	}
	if changed == 0 {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, reason)
		VALUES (?, 'administrator', ?, 'agent.project.joined', 'agent', ?, 'project scope granted')`, projectID, actorID, agentID); err != nil {
		return fmt.Errorf("audit added agent project membership: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent project membership: %w", err)
	}
	return nil
}

func (r *AgentRepository) RemoveProject(ctx context.Context, agentID, projectID, actorID string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent project membership removal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM agent_projects WHERE agent_id = ? AND project_id = ?`, agentID, projectID)
	if err != nil {
		return fmt.Errorf("remove agent project membership: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count removed agent project membership: %w", err)
	}
	if changed == 0 {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, reason)
		VALUES (?, 'administrator', ?, 'agent.project.left', 'agent', ?, 'project scope revoked')`, projectID, actorID, agentID); err != nil {
		return fmt.Errorf("audit removed agent project membership: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent project membership removal: %w", err)
	}
	return nil
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func validateAgentCredential(ctx context.Context, tx *Transaction, snapshot agent.Snapshot) error {
	if snapshot.Runtime != agent.RuntimeAPI {
		return nil
	}
	var matches bool
	err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM credentials WHERE id = ? AND lower(provider) = ?)", snapshot.CredentialID, snapshot.Provider).Scan(&matches)
	if err != nil {
		return fmt.Errorf("validate agent credential reference: %w", err)
	}
	if !matches {
		return agent.ErrCredentialMismatch
	}
	return nil
}

func insertAgent(ctx context.Context, executor sqlExecutor, snapshot agent.Snapshot) error {
	_, err := executor.ExecContext(ctx, `INSERT INTO agents (id, name, avatar, description, role, runtime, provider, provider_base_url, model, credential_id, instructions, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?)`, snapshot.ID, snapshot.Name, snapshot.Avatar, snapshot.Description,
		snapshot.Role, snapshot.Runtime, snapshot.Provider, snapshot.ProviderBaseURL, snapshot.Model, snapshot.CredentialID, snapshot.Instructions, snapshot.Status)
	return err
}

func insertAgentRelations(ctx context.Context, tx *Transaction, snapshot agent.Snapshot) error {
	if err := insertAgentLanguages(ctx, tx, snapshot); err != nil {
		return err
	}
	for _, capability := range snapshot.Capabilities {
		if _, err := tx.ExecContext(ctx, "INSERT INTO agent_capabilities(agent_id, capability) VALUES (?, ?)", snapshot.ID, capability); err != nil {
			return fmt.Errorf("insert agent capability: %w", err)
		}
	}
	return nil
}

func insertAgentLanguages(ctx context.Context, tx *Transaction, snapshot agent.Snapshot) error {
	for position, language := range snapshot.LanguagePreferences {
		if _, err := tx.ExecContext(ctx, "INSERT INTO agent_languages(agent_id, position, language) VALUES (?, ?, ?)", snapshot.ID, position, language); err != nil {
			return fmt.Errorf("insert agent language: %w", err)
		}
	}
	return nil
}

func syncAgentCapabilities(ctx context.Context, tx *Transaction, snapshot agent.Snapshot) error {
	rows, err := tx.QueryContext(ctx, "SELECT capability FROM agent_capabilities WHERE agent_id = ?", snapshot.ID)
	if err != nil {
		return fmt.Errorf("query current agent capabilities: %w", err)
	}
	current := make(map[string]struct{})
	for rows.Next() {
		var capability string
		if err := rows.Scan(&capability); err != nil {
			return closeRowsAfter(rows, "current agent capabilities", fmt.Errorf("scan current agent capability: %w", err))
		}
		current[capability] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return closeRowsAfter(rows, "current agent capabilities", fmt.Errorf("iterate current agent capabilities: %w", err))
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close current agent capabilities: %w", err)
	}

	desired := make(map[string]struct{}, len(snapshot.Capabilities))
	for _, capability := range snapshot.Capabilities {
		desired[capability] = struct{}{}
	}
	for capability := range current {
		if _, keep := desired[capability]; keep {
			continue
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM agent_capabilities WHERE agent_id = ? AND capability = ?", snapshot.ID, capability); err != nil {
			return fmt.Errorf("remove agent capability: %w", err)
		}
	}
	for capability := range desired {
		if _, exists := current[capability]; exists {
			continue
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO agent_capabilities(agent_id, capability) VALUES (?, ?)", snapshot.ID, capability); err != nil {
			return fmt.Errorf("add agent capability: %w", err)
		}
	}
	return nil
}

func loadAgentRelations(ctx context.Context, database *Database, snapshot *agent.Snapshot) error {
	languages, err := database.QueryContext(ctx, "SELECT language FROM agent_languages WHERE agent_id = ? ORDER BY position", snapshot.ID)
	if err != nil {
		return fmt.Errorf("query agent languages: %w", err)
	}
	for languages.Next() {
		var value string
		if err := languages.Scan(&value); err != nil {
			return closeRowsAfter(languages, "agent languages", fmt.Errorf("scan agent language: %w", err))
		}
		snapshot.LanguagePreferences = append(snapshot.LanguagePreferences, value)
	}
	if err := languages.Err(); err != nil {
		return closeRowsAfter(languages, "agent languages", fmt.Errorf("iterate agent languages: %w", err))
	}
	if err := languages.Close(); err != nil {
		return fmt.Errorf("close agent languages: %w", err)
	}
	capabilities, err := database.QueryContext(ctx, "SELECT capability FROM agent_capabilities WHERE agent_id = ? ORDER BY capability", snapshot.ID)
	if err != nil {
		return fmt.Errorf("query agent capabilities: %w", err)
	}
	for capabilities.Next() {
		var value string
		if err := capabilities.Scan(&value); err != nil {
			return closeRowsAfter(capabilities, "agent capabilities", fmt.Errorf("scan agent capability: %w", err))
		}
		snapshot.Capabilities = append(snapshot.Capabilities, value)
	}
	if err := capabilities.Err(); err != nil {
		return closeRowsAfter(capabilities, "agent capabilities", fmt.Errorf("iterate agent capabilities: %w", err))
	}
	if err := capabilities.Close(); err != nil {
		return fmt.Errorf("close agent capabilities: %w", err)
	}
	projects, err := database.QueryContext(ctx, "SELECT project_id FROM agent_projects WHERE agent_id = ? ORDER BY project_id", snapshot.ID)
	if err != nil {
		return fmt.Errorf("query agent projects: %w", err)
	}
	for projects.Next() {
		var value string
		if err := projects.Scan(&value); err != nil {
			return closeRowsAfter(projects, "agent projects", fmt.Errorf("scan agent project: %w", err))
		}
		snapshot.ProjectIDs = append(snapshot.ProjectIDs, value)
	}
	if err := projects.Err(); err != nil {
		return closeRowsAfter(projects, "agent projects", fmt.Errorf("iterate agent projects: %w", err))
	}
	if err := projects.Close(); err != nil {
		return fmt.Errorf("close agent projects: %w", err)
	}
	return nil
}

type agentRowScanner interface{ Scan(...any) error }

const agentSelect = `SELECT id, name, avatar, description, role, runtime, provider, provider_base_url, model, COALESCE(credential_id, ''), instructions, status FROM agents`

func scanAgent(row agentRowScanner) (agent.Snapshot, error) {
	var value agent.Snapshot
	err := row.Scan(&value.ID, &value.Name, &value.Avatar, &value.Description, &value.Role, &value.Runtime,
		&value.Provider, &value.ProviderBaseURL, &value.Model, &value.CredentialID, &value.Instructions, &value.Status)
	return value, err
}
