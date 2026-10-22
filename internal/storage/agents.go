package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"boAIrd/internal/agent"
)

type AgentRepository struct{ database *sql.DB }

func NewAgentRepository(database *sql.DB) (*AgentRepository, error) {
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
	defer tx.Rollback()
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
	rows, err := r.database.QueryContext(ctx, agentSelect+" ORDER BY name COLLATE NOCASE, id LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query agents: %w", err)
	}
	snapshots := make([]agent.Snapshot, 0)
	for rows.Next() {
		snapshot, err := scanAgent(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate agents: %w", err)
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
	defer tx.Rollback()
	if err := validateAgentCredential(ctx, tx, snapshot); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE agents SET name = ?, avatar = ?, description = ?, role = ?, runtime = ?,
		provider = ?, provider_base_url = ?, model = ?, credential_id = NULLIF(?, ''), instructions = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, snapshot.Name, snapshot.Avatar, snapshot.Description, snapshot.Role, snapshot.Runtime,
		snapshot.Provider, snapshot.ProviderBaseURL, snapshot.Model, snapshot.CredentialID, snapshot.Instructions, snapshot.Status, snapshot.ID)
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
	for _, table := range []string{"agent_languages", "agent_capabilities"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE agent_id = ?", snapshot.ID); err != nil {
			return fmt.Errorf("replace agent relations: %w", err)
		}
	}
	if err := insertAgentRelations(ctx, tx, snapshot); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent update: %w", err)
	}
	return nil
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func validateAgentCredential(ctx context.Context, tx *sql.Tx, snapshot agent.Snapshot) error {
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

func insertAgentRelations(ctx context.Context, tx *sql.Tx, snapshot agent.Snapshot) error {
	for position, language := range snapshot.LanguagePreferences {
		if _, err := tx.ExecContext(ctx, "INSERT INTO agent_languages(agent_id, position, language) VALUES (?, ?, ?)", snapshot.ID, position, language); err != nil {
			return fmt.Errorf("insert agent language: %w", err)
		}
	}
	for _, capability := range snapshot.Capabilities {
		if _, err := tx.ExecContext(ctx, "INSERT INTO agent_capabilities(agent_id, capability) VALUES (?, ?)", snapshot.ID, capability); err != nil {
			return fmt.Errorf("insert agent capability: %w", err)
		}
	}
	return nil
}

func loadAgentRelations(ctx context.Context, database *sql.DB, snapshot *agent.Snapshot) error {
	languages, err := database.QueryContext(ctx, "SELECT language FROM agent_languages WHERE agent_id = ? ORDER BY position", snapshot.ID)
	if err != nil {
		return fmt.Errorf("query agent languages: %w", err)
	}
	for languages.Next() {
		var value string
		if err := languages.Scan(&value); err != nil {
			languages.Close()
			return fmt.Errorf("scan agent language: %w", err)
		}
		snapshot.LanguagePreferences = append(snapshot.LanguagePreferences, value)
	}
	if err := languages.Err(); err != nil {
		languages.Close()
		return fmt.Errorf("iterate agent languages: %w", err)
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
			capabilities.Close()
			return fmt.Errorf("scan agent capability: %w", err)
		}
		snapshot.Capabilities = append(snapshot.Capabilities, value)
	}
	if err := capabilities.Err(); err != nil {
		capabilities.Close()
		return fmt.Errorf("iterate agent capabilities: %w", err)
	}
	if err := capabilities.Close(); err != nil {
		return fmt.Errorf("close agent capabilities: %w", err)
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
