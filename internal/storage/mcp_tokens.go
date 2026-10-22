package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"boAIrd/internal/mcptoken"
)

type MCPTokenRepository struct{ database *sql.DB }

func NewMCPTokenRepository(database *sql.DB) (*MCPTokenRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &MCPTokenRepository{database: database}, nil
}

func (r *MCPTokenRepository) Create(ctx context.Context, value mcptoken.Record, actorID string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin MCP token creation: %w", err)
	}
	defer tx.Rollback()
	if err := insertMCPToken(ctx, tx, value); err != nil {
		return err
	}
	if err := insertMCPTokenEvent(ctx, tx, value, "", actorID, "token.issued", value.IssuedAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit MCP token creation: %w", err)
	}
	return nil
}

func (r *MCPTokenRepository) Rotate(ctx context.Context, previousID string, value mcptoken.Record, actorID string, now time.Time) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin MCP token rotation: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE mcp_agent_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL AND expires_at > ?`, formatTimestamp(now), previousID, formatTimestamp(now))
	if err != nil {
		return fmt.Errorf("revoke previous MCP token: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check previous MCP token: %w", err)
	}
	if count != 1 {
		return mcptoken.ErrInactive
	}
	if err := insertMCPToken(ctx, tx, value); err != nil {
		return err
	}
	if err := insertMCPTokenEvent(ctx, tx, value, previousID, actorID, "token.rotated", now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit MCP token rotation: %w", err)
	}
	return nil
}

func (r *MCPTokenRepository) Revoke(ctx context.Context, id, actorID string, now time.Time) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin MCP token revocation: %w", err)
	}
	defer tx.Rollback()
	var value mcptoken.Record
	if err := scanMCPToken(tx.QueryRowContext(ctx, `SELECT id, agent_id, token_hash, issued_at, expires_at, revoked_at FROM mcp_agent_tokens WHERE id = ?`, id), &value); err != nil {
		return err
	}
	if value.RevokedAt != nil {
		return mcptoken.ErrInactive
	}
	if _, err := tx.ExecContext(ctx, `UPDATE mcp_agent_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, formatTimestamp(now), id); err != nil {
		return fmt.Errorf("revoke MCP token: %w", err)
	}
	value.RevokedAt = &now
	if err := insertMCPTokenEvent(ctx, tx, value, "", actorID, "token.revoked", now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit MCP token revocation: %w", err)
	}
	return nil
}

func (r *MCPTokenRepository) Get(ctx context.Context, id string) (mcptoken.Record, error) {
	var value mcptoken.Record
	if err := scanMCPToken(r.database.QueryRowContext(ctx, `SELECT id, agent_id, token_hash, issued_at, expires_at, revoked_at FROM mcp_agent_tokens WHERE id = ?`, id), &value); err != nil {
		return mcptoken.Record{}, err
	}
	return value, nil
}

func (r *MCPTokenRepository) List(ctx context.Context) ([]mcptoken.Record, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, agent_id, token_hash, issued_at, expires_at, revoked_at FROM mcp_agent_tokens ORDER BY issued_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("list MCP tokens: %w", err)
	}
	defer rows.Close()
	values := make([]mcptoken.Record, 0)
	for rows.Next() {
		var value mcptoken.Record
		if err := scanMCPToken(rows, &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate MCP tokens: %w", err)
	}
	return values, nil
}

func (r *MCPTokenRepository) VerificationRecords(ctx context.Context) ([]mcptoken.Record, error) {
	return r.List(ctx)
}

func (r *MCPTokenRepository) Bootstrap(ctx context.Context, values []mcptoken.Record, actorID string) (bool, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin MCP token bootstrap: %w", err)
	}
	defer tx.Rollback()
	var completed string
	err = tx.QueryRowContext(ctx, `SELECT value FROM app_metadata WHERE key = 'mcp_agent_tokens_bootstrapped'`).Scan(&completed)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("check MCP token bootstrap: %w", err)
	}
	for _, value := range values {
		if err := insertMCPToken(ctx, tx, value); err != nil {
			return false, err
		}
		if err := insertMCPTokenEvent(ctx, tx, value, "", actorID, "token.bootstrapped", value.IssuedAt); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_metadata(key, value) VALUES ('mcp_agent_tokens_bootstrapped', 'true')`); err != nil {
		return false, fmt.Errorf("record MCP token bootstrap: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit MCP token bootstrap: %w", err)
	}
	return true, nil
}

type mcpTokenExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertMCPToken(ctx context.Context, execer mcpTokenExecer, value mcptoken.Record) error {
	_, err := execer.ExecContext(ctx, `INSERT INTO mcp_agent_tokens(id, agent_id, token_hash, issued_at, expires_at) VALUES (?, ?, ?, ?, ?)`, value.ID, value.AgentID, value.Digest[:], formatTimestamp(value.IssuedAt), formatTimestamp(value.ExpiresAt))
	if err != nil {
		return fmt.Errorf("insert MCP token: %w", err)
	}
	return nil
}

func insertMCPTokenEvent(ctx context.Context, execer mcpTokenExecer, value mcptoken.Record, relatedID, actorID, eventType string, createdAt time.Time) error {
	_, err := execer.ExecContext(ctx, `INSERT INTO mcp_agent_token_events(token_id, related_token_id, agent_id, actor_id, event_type, created_at) VALUES (?, ?, ?, ?, ?, ?)`, value.ID, relatedID, value.AgentID, actorID, eventType, formatTimestamp(createdAt))
	if err != nil {
		return fmt.Errorf("insert MCP token audit event: %w", err)
	}
	return nil
}

type mcpTokenScanner interface {
	Scan(...any) error
}

func scanMCPToken(scanner mcpTokenScanner, value *mcptoken.Record) error {
	var digest []byte
	var issuedAt, expiresAt string
	var revokedAt sql.NullString
	if err := scanner.Scan(&value.ID, &value.AgentID, &digest, &issuedAt, &expiresAt, &revokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcptoken.ErrNotFound
		}
		return fmt.Errorf("scan MCP token: %w", err)
	}
	if len(digest) != len(value.Digest) {
		return errors.New("stored MCP token verifier has invalid length")
	}
	copy(value.Digest[:], digest)
	var err error
	if value.IssuedAt, err = time.Parse(time.RFC3339Nano, issuedAt); err != nil {
		return fmt.Errorf("parse MCP token issue time: %w", err)
	}
	if value.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt); err != nil {
		return fmt.Errorf("parse MCP token expiry time: %w", err)
	}
	if revokedAt.Valid {
		value.RevokedAt = new(time.Time)
		if *value.RevokedAt, err = time.Parse(time.RFC3339Nano, revokedAt.String); err != nil {
			return fmt.Errorf("parse MCP token revocation time: %w", err)
		}
	}
	return nil
}

func formatTimestamp(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
