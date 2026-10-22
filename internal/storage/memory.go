package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"boAIrd/internal/memory"
)

type MemoryRepository struct{ database *sql.DB }

func NewMemoryRepository(database *sql.DB) (*MemoryRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &MemoryRepository{database: database}, nil
}

func (r *MemoryRepository) Create(ctx context.Context, value memory.Summary) error {
	snapshot := value.Snapshot()
	sources, err := json.Marshal(snapshot.SourceIDs)
	if err != nil {
		return fmt.Errorf("encode memory source IDs: %w", err)
	}
	if _, err := r.database.ExecContext(ctx, `INSERT INTO project_memory(id, project_id, category, version, content, source_ids, created_by_type, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, snapshot.ID, snapshot.ProjectID, snapshot.Category, snapshot.Version,
		snapshot.Content, string(sources), snapshot.CreatedByType, snapshot.CreatedBy, formatDatabaseTime(snapshot.CreatedAt)); err != nil {
		return fmt.Errorf("insert project memory: %w", err)
	}
	return nil
}

func (r *MemoryRepository) List(ctx context.Context, projectID string) ([]memory.Summary, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, project_id, category, version, content, source_ids, created_by_type, created_by, created_at
		FROM project_memory WHERE project_id = ? ORDER BY category, version`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query project memory: %w", err)
	}
	values := make([]memory.Summary, 0)
	for rows.Next() {
		var snapshot memory.Snapshot
		var sources, created string
		if err := rows.Scan(&snapshot.ID, &snapshot.ProjectID, &snapshot.Category, &snapshot.Version, &snapshot.Content,
			&sources, &snapshot.CreatedByType, &snapshot.CreatedBy, &created); err != nil {
			return nil, closeRowsAfter(rows, "project memory", fmt.Errorf("scan project memory: %w", err))
		}
		if err := json.Unmarshal([]byte(sources), &snapshot.SourceIDs); err != nil {
			return nil, closeRowsAfter(rows, "project memory", fmt.Errorf("decode memory source IDs: %w", err))
		}
		createdAt, err := parseDatabaseTime(created)
		if err != nil {
			return nil, closeRowsAfter(rows, "project memory", fmt.Errorf("parse memory creation time: %w", err))
		}
		snapshot.CreatedAt = createdAt
		value, err := memory.Restore(snapshot)
		if err != nil {
			return nil, closeRowsAfter(rows, "project memory", fmt.Errorf("restore project memory: %w", err))
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "project memory", fmt.Errorf("iterate project memory: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close project memory rows: %w", err)
	}
	return values, nil
}

func (r *MemoryRepository) MaxVersion(ctx context.Context, projectID string, category memory.Category) (int, error) {
	var version int
	if err := r.database.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM project_memory WHERE project_id = ? AND category = ?`,
		projectID, category).Scan(&version); err != nil {
		return 0, fmt.Errorf("read memory version: %w", err)
	}
	return version, nil
}

func (r *MemoryRepository) DeleteOlderThanVersion(ctx context.Context, projectID string, category memory.Category, version int) (int, error) {
	result, err := r.database.ExecContext(ctx, `DELETE FROM project_memory WHERE project_id = ? AND category = ? AND version < ?`,
		projectID, category, version)
	if err != nil {
		return 0, fmt.Errorf("prune project memory: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned project memory: %w", err)
	}
	return int(count), nil
}
