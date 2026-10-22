package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"boAIrd/internal/run"
)

type RunRepository struct{ database *sql.DB }

func NewRunRepository(database *sql.DB) (*RunRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &RunRepository{database: database}, nil
}

func (r *RunRepository) Create(ctx context.Context, value run.Run) error {
	snapshot := value.Snapshot()
	_, err := r.database.ExecContext(ctx, `INSERT INTO runs (
		id, project_id, task_id, agent_id, status, started_at, finished_at, runtime, provider, model, input, output, error
	) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, snapshot.ID, snapshot.ProjectID, snapshot.TaskID,
		snapshot.AgentID, snapshot.Status, nullableTime(snapshot.StartedAt), nullableTime(snapshot.FinishedAt), snapshot.Runtime,
		snapshot.Provider, snapshot.Model, snapshot.Input, snapshot.Output, snapshot.Error)
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}
	return nil
}

func (r *RunRepository) Update(ctx context.Context, value run.Run) error {
	snapshot := value.Snapshot()
	result, err := r.database.ExecContext(ctx, `UPDATE runs SET status = ?, started_at = ?, finished_at = ?, output = ?, error = ? WHERE id = ?`,
		snapshot.Status, nullableTime(snapshot.StartedAt), nullableTime(snapshot.FinishedAt), snapshot.Output, snapshot.Error, snapshot.ID)
	if err != nil {
		return fmt.Errorf("update run: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated runs: %w", err)
	}
	if count == 0 {
		return run.ErrNotFound
	}
	return nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
