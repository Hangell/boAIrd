package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"boAIrd/internal/credential"
)

type CredentialRepository struct {
	database *Database
}

func NewCredentialRepository(database *Database) (*CredentialRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &CredentialRepository{database: database}, nil
}

func (r *CredentialRepository) Create(ctx context.Context, record credential.Record) error {
	_, err := r.database.ExecContext(ctx, `INSERT INTO credentials (id, name, provider, secret_ciphertext)
		VALUES (?, ?, ?, ?)`, record.ID, record.Name, record.Provider, record.SecretCiphertext)
	if err != nil {
		return fmt.Errorf("insert encrypted credential: %w", err)
	}
	return nil
}

func (r *CredentialRepository) Get(ctx context.Context, id string) (credential.Record, error) {
	var record credential.Record
	err := r.database.QueryRowContext(ctx, `SELECT id, name, provider, secret_ciphertext
		FROM credentials WHERE id = ?`, id).Scan(&record.ID, &record.Name, &record.Provider, &record.SecretCiphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return credential.Record{}, credential.ErrNotFound
	}
	if err != nil {
		return credential.Record{}, fmt.Errorf("read encrypted credential: %w", err)
	}
	return record, nil
}

func (r *CredentialRepository) GetForUse(ctx context.Context, id, agentID, projectID string) (credential.Record, error) {
	var record credential.Record
	err := r.database.QueryRowContext(ctx, `SELECT c.id, c.name, c.provider, c.secret_ciphertext
		FROM credentials c JOIN agents a ON a.credential_id=c.id
		WHERE c.id=? AND a.id=? AND lower(c.provider)=lower(a.provider) AND (
			EXISTS (SELECT 1 FROM agent_projects ap WHERE ap.agent_id=a.id AND ap.project_id=?)
			OR EXISTS (
				SELECT 1 FROM brain_configuration bc
				JOIN brains b ON b.id=bc.brain_id AND b.active=1
				JOIN projects p ON p.id=b.project_id AND p.brain_id=b.id
				WHERE bc.agent_id=a.id AND p.id=?
			)
		)`, id, agentID, projectID, projectID).Scan(&record.ID, &record.Name, &record.Provider, &record.SecretCiphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return credential.Record{}, credential.ErrCredentialUnavailable
	}
	if err != nil {
		return credential.Record{}, fmt.Errorf("authorize credential resolution: %w", err)
	}
	return record, nil
}

// BindIntegration grants one MCP Agent access to a purpose-specific credential
// in a project and records the authority change in the same transaction.
func (r *CredentialRepository) BindIntegration(ctx context.Context, agentID, projectID, purpose, credentialID, actorID string) error {
	if ctx == nil || agentID == "" || projectID == "" || purpose != credential.ProviderGitHub || credentialID == "" || actorID == "" {
		return credential.ErrCredentialUnavailable
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin integration credential binding: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var available bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM credentials WHERE id = ? AND lower(provider) = ?
	)`, credentialID, purpose).Scan(&available); err != nil {
		return fmt.Errorf("validate integration credential provider: %w", err)
	}
	if !available {
		return credential.ErrCredentialUnavailable
	}
	var scoped bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM agents a
		JOIN projects p ON p.id = ? AND p.status <> 'ARCHIVED'
		WHERE a.id = ? AND a.runtime = 'MCP' AND (
			EXISTS (SELECT 1 FROM agent_projects ap WHERE ap.agent_id = a.id AND ap.project_id = p.id)
			OR EXISTS (
				SELECT 1 FROM brain_configuration bc
				JOIN brains b ON b.id = bc.brain_id AND b.active = 1
				WHERE bc.agent_id = a.id AND b.project_id = p.id AND p.brain_id = b.id
			)
		)
	)`, projectID, agentID).Scan(&scoped); err != nil {
		return fmt.Errorf("validate integration credential scope: %w", err)
	}
	if !scoped {
		return credential.ErrCredentialUnavailable
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_integration_credentials(agent_id, project_id, purpose, credential_id)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(agent_id, project_id, purpose) DO UPDATE SET credential_id = excluded.credential_id`,
		agentID, projectID, purpose, credentialID); err != nil {
		return fmt.Errorf("persist integration credential binding: %w", err)
	}
	if err := recordIntegrationCredentialAudit(ctx, tx, projectID, agentID, actorID, "credential.integration.bound"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit integration credential binding: %w", err)
	}
	return nil
}

// UnbindIntegration revokes one purpose-specific project grant and audits the
// removal atomically. Missing grants are indistinguishable from unavailable ones.
func (r *CredentialRepository) UnbindIntegration(ctx context.Context, agentID, projectID, purpose, actorID string) error {
	if ctx == nil || agentID == "" || projectID == "" || purpose != credential.ProviderGitHub || actorID == "" {
		return credential.ErrCredentialUnavailable
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin integration credential unbinding: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM agent_integration_credentials
		WHERE agent_id = ? AND project_id = ? AND purpose = ?`, agentID, projectID, purpose)
	if err != nil {
		return fmt.Errorf("remove integration credential binding: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count removed integration credential bindings: %w", err)
	}
	if count == 0 {
		return credential.ErrCredentialUnavailable
	}
	if err := recordIntegrationCredentialAudit(ctx, tx, projectID, agentID, actorID, "credential.integration.unbound"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit integration credential unbinding: %w", err)
	}
	return nil
}

// GetIntegrationForUse returns ciphertext only when the current MCP Agent,
// project scope, and credential provider all match the persisted grant.
func (r *CredentialRepository) GetIntegrationForUse(ctx context.Context, agentID, projectID, purpose string) (credential.Record, error) {
	if ctx == nil || agentID == "" || projectID == "" || purpose != credential.ProviderGitHub {
		return credential.Record{}, credential.ErrCredentialUnavailable
	}
	var record credential.Record
	err := r.database.QueryRowContext(ctx, `SELECT c.id, c.name, c.provider, c.secret_ciphertext
		FROM agent_integration_credentials i
		JOIN credentials c ON c.id = i.credential_id AND lower(c.provider) = lower(i.purpose)
		JOIN agents a ON a.id = i.agent_id AND a.runtime = 'MCP'
		JOIN projects p ON p.id = i.project_id AND p.status <> 'ARCHIVED'
		WHERE i.agent_id = ? AND i.project_id = ? AND i.purpose = ? AND (
			EXISTS (SELECT 1 FROM agent_projects ap WHERE ap.agent_id = a.id AND ap.project_id = p.id)
			OR EXISTS (
				SELECT 1 FROM brain_configuration bc
				JOIN brains b ON b.id = bc.brain_id AND b.active = 1
				WHERE bc.agent_id = a.id AND b.project_id = p.id AND p.brain_id = b.id
			)
		)`, agentID, projectID, purpose).Scan(&record.ID, &record.Name, &record.Provider, &record.SecretCiphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return credential.Record{}, credential.ErrCredentialUnavailable
	}
	if err != nil {
		return credential.Record{}, fmt.Errorf("authorize integration credential resolution: %w", err)
	}
	return record, nil
}

func recordIntegrationCredentialAudit(ctx context.Context, tx *Transaction, projectID, agentID, actorID, action string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id, task_id, actor_type, actor_id, event_type, resource_type, resource_id, result, reason)
		VALUES (?, NULL, 'administrator', ?, ?, 'agent_integration_credential', ?, 'success', 'purpose=github')`,
		projectID, actorID, action, agentID); err != nil {
		return fmt.Errorf("audit integration credential change: %w", err)
	}
	return nil
}

func (r *CredentialRepository) List(ctx context.Context) ([]credential.Record, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, name, provider FROM credentials ORDER BY lower(name), id`)
	if err != nil {
		return nil, fmt.Errorf("query credential metadata: %w", err)
	}

	records := make([]credential.Record, 0)
	for rows.Next() {
		var record credential.Record
		if err := rows.Scan(&record.ID, &record.Name, &record.Provider); err != nil {
			return nil, closeRowsAfter(rows, "credential metadata", fmt.Errorf("scan credential metadata: %w", err))
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "credential metadata", fmt.Errorf("iterate credential metadata: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close credential metadata rows: %w", err)
	}
	return records, nil
}

func (r *CredentialRepository) Delete(ctx context.Context, id string) error {
	result, err := r.database.ExecContext(ctx, "DELETE FROM credentials WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete encrypted credential: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted credentials: %w", err)
	}
	if count == 0 {
		return credential.ErrNotFound
	}
	return nil
}
