package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"boAIrd/internal/project"
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
	tools, err := json.Marshal(snapshot.ToolsUsed)
	if err != nil {
		return fmt.Errorf("encode run tools: %w", err)
	}
	_, err = r.database.ExecContext(ctx, `INSERT INTO runs (
		id, project_id, task_id, agent_id, status, started_at, finished_at, runtime, provider, model, input, output, error, tools_used, kind
	) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, snapshot.ID, snapshot.ProjectID, snapshot.TaskID,
		snapshot.AgentID, snapshot.Status, nullableTime(snapshot.StartedAt), nullableTime(snapshot.FinishedAt), snapshot.Runtime,
		snapshot.Provider, snapshot.Model, snapshot.Input, snapshot.Output, snapshot.Error, string(tools), snapshot.Kind)
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}
	return nil
}

func (r *RunRepository) Update(ctx context.Context, value run.Run) error {
	snapshot := value.Snapshot()
	tools, err := json.Marshal(snapshot.ToolsUsed)
	if err != nil {
		return fmt.Errorf("encode run tools: %w", err)
	}
	result, err := r.database.ExecContext(ctx, `UPDATE runs SET status = ?, started_at = ?, finished_at = ?, output = ?, error = ?, tools_used = ? WHERE id = ?`,
		snapshot.Status, nullableTime(snapshot.StartedAt), nullableTime(snapshot.FinishedAt), snapshot.Output, snapshot.Error, string(tools), snapshot.ID)
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

func (r *RunRepository) Get(ctx context.Context, id string) (run.Run, error) {
	value, err := scanRun(r.database.QueryRowContext(ctx, runSelect+" WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return run.Run{}, run.ErrNotFound
	}
	if err != nil {
		return run.Run{}, fmt.Errorf("read run: %w", err)
	}
	return value, nil
}

func (r *RunRepository) List(ctx context.Context, projectID, taskID, agentID string, limit, offset int) ([]run.Run, error) {
	var projectExists bool
	if err := r.database.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM projects WHERE id=?)", projectID).Scan(&projectExists); err != nil {
		return nil, fmt.Errorf("check run project: %w", err)
	}
	if !projectExists {
		return nil, project.ErrNotFound
	}
	query := runSelect + " WHERE project_id = ?"
	args := []any{projectID}
	if taskID != "" {
		query += " AND task_id = ?"
		args = append(args, taskID)
	}
	if agentID != "" {
		query += " AND agent_id = ?"
		args = append(args, agentID)
	}
	query += " ORDER BY created_at DESC, id LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := r.database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query runs: %w", err)
	}
	defer rows.Close()
	values := make([]run.Run, 0)
	for rows.Next() {
		value, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}
	return values, nil
}

// CountWorkerAttempts returns the number of worker execution Runs for a task,
// excluding Brain planning and failure-analysis Runs linked to the same task.
func (r *RunRepository) CountWorkerAttempts(ctx context.Context, projectID, taskID string) (int, error) {
	var count int
	if err := r.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE project_id = ? AND task_id = ? AND kind = 'worker'`,
		projectID, taskID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count worker attempts: %w", err)
	}
	return count, nil
}

func (r *RunRepository) ListAuditEvents(ctx context.Context, runID string) ([]run.AuditEvent, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, project_id, COALESCE(task_id,''), actor_type, actor_id,
		event_type, resource_type, resource_id, from_status, to_status, created_at
		FROM audit_events WHERE resource_type='run' AND resource_id=? ORDER BY sequence`, runID)
	if err != nil {
		return nil, fmt.Errorf("query run audit events: %w", err)
	}
	defer rows.Close()
	events := make([]run.AuditEvent, 0)
	for rows.Next() {
		var event run.AuditEvent
		var created string
		if err := rows.Scan(&event.ID, &event.ProjectID, &event.TaskID, &event.ActorType, &event.ActorID, &event.EventType,
			&event.ResourceType, &event.ResourceID, &event.FromStatus, &event.ToStatus, &created); err != nil {
			return nil, fmt.Errorf("scan run audit event: %w", err)
		}
		event.CreatedAt, err = parseDatabaseTime(created)
		if err != nil {
			return nil, fmt.Errorf("parse run audit event time: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run audit events: %w", err)
	}
	return events, nil
}

func (r *RunRepository) ListAuditEventsForRuns(ctx context.Context, runIDs []string) (map[string][]run.AuditEvent, error) {
	eventsByRun := make(map[string][]run.AuditEvent, len(runIDs))
	if len(runIDs) == 0 {
		return eventsByRun, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(runIDs)), ",")
	args := make([]any, len(runIDs))
	for index, id := range runIDs {
		args[index] = id
		eventsByRun[id] = make([]run.AuditEvent, 0)
	}
	rows, err := r.database.QueryContext(ctx, `SELECT id, project_id, COALESCE(task_id,''), actor_type, actor_id,
		event_type, resource_type, resource_id, from_status, to_status, created_at
		FROM audit_events WHERE resource_type='run' AND resource_id IN (`+placeholders+`) ORDER BY resource_id, sequence`, args...)
	if err != nil {
		return nil, fmt.Errorf("query run audit events: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var event run.AuditEvent
		var created string
		if err := rows.Scan(&event.ID, &event.ProjectID, &event.TaskID, &event.ActorType, &event.ActorID, &event.EventType,
			&event.ResourceType, &event.ResourceID, &event.FromStatus, &event.ToStatus, &created); err != nil {
			return nil, fmt.Errorf("scan run audit event: %w", err)
		}
		event.CreatedAt, err = parseDatabaseTime(created)
		if err != nil {
			return nil, fmt.Errorf("parse run audit event time: %w", err)
		}
		eventsByRun[event.ResourceID] = append(eventsByRun[event.ResourceID], event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run audit events: %w", err)
	}
	return eventsByRun, nil
}

type runRowScanner interface{ Scan(...any) error }

const runSelect = `SELECT id, project_id, COALESCE(task_id,''), agent_id, status,
	COALESCE(started_at,''), COALESCE(finished_at,''), runtime, provider, model, input, output, error, tools_used, kind
	FROM runs`

func scanRun(row runRowScanner) (run.Run, error) {
	var snapshot run.Snapshot
	var started, finished, toolsJSON string
	if err := row.Scan(&snapshot.ID, &snapshot.ProjectID, &snapshot.TaskID, &snapshot.AgentID, &snapshot.Status,
		&started, &finished, &snapshot.Runtime, &snapshot.Provider, &snapshot.Model, &snapshot.Input, &snapshot.Output,
		&snapshot.Error, &toolsJSON, &snapshot.Kind); err != nil {
		return run.Run{}, err
	}
	var err error
	if started != "" {
		snapshot.StartedAt, err = parseDatabaseTime(started)
		if err != nil {
			return run.Run{}, err
		}
	}
	if finished != "" {
		snapshot.FinishedAt, err = parseDatabaseTime(finished)
		if err != nil {
			return run.Run{}, err
		}
	}
	if err := json.Unmarshal([]byte(toolsJSON), &snapshot.ToolsUsed); err != nil {
		return run.Run{}, fmt.Errorf("decode run tools: %w", err)
	}
	return run.Restore(snapshot)
}

func parseDatabaseTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid database timestamp %q", strings.TrimSpace(value))
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
