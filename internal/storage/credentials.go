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

func (r *CredentialRepository) List(ctx context.Context) ([]credential.Record, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, name, provider FROM credentials ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("query credential metadata: %w", err)
	}
	defer rows.Close()

	records := make([]credential.Record, 0)
	for rows.Next() {
		var record credential.Record
		if err := rows.Scan(&record.ID, &record.Name, &record.Provider); err != nil {
			return nil, fmt.Errorf("scan credential metadata: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate credential metadata: %w", err)
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
