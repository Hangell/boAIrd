package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"boAIrd/internal/credential"
)

type CredentialRepository struct {
	database *sql.DB
}

func NewCredentialRepository(database *sql.DB) (*CredentialRepository, error) {
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

func (r *CredentialRepository) List(ctx context.Context) ([]credential.Record, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, name, provider FROM credentials ORDER BY name COLLATE NOCASE, id`)
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
