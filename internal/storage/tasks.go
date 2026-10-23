package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"boAIrd/internal/task"
)

type TaskRepository struct{ database *Database }

func NewTaskRepository(database *Database) (*TaskRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &TaskRepository{database: database}, nil
}

func (r *TaskRepository) RecordAuthorizationDenial(ctx context.Context, actorID, projectID, required, resource, reason string) error {
	_, err := r.database.ExecContext(ctx, `INSERT INTO authorization_denials(principal_id, project_id, required_capability, resource, reason)
		VALUES (?, ?, ?, ?, ?)`, actorID, projectID, required, resource, reason)
	if err != nil {
		return fmt.Errorf("insert authorization denial: %w", err)
	}
	return nil
}

func (r *TaskRepository) Create(ctx context.Context, value task.Task) error {
	if err := writeTask(ctx, r.database, value); err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	return nil
}

func (r *TaskRepository) Get(ctx context.Context, id string) (task.Task, error) {
	value, err := scanTask(r.database.QueryRowContext(ctx, taskSelect+" WHERE id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, task.ErrNotFound
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("read task: %w", err)
	}
	return value, nil
}

func (r *TaskRepository) List(ctx context.Context, projectID string, status task.Status, assignedTo string, limit, offset int) ([]task.Task, error) {
	var exists bool
	if err := r.database.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM projects WHERE id=?)", projectID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check task project: %w", err)
	}
	if !exists {
		return nil, task.ErrProjectNotFound
	}
	query, args := taskSelect+" WHERE project_id=?", []any{projectID}
	if status != "" {
		query += " AND status=?"
		args = append(args, status)
	}
	if assignedTo != "" {
		query += " AND assigned_to=?"
		args = append(args, assignedTo)
	}
	query += " ORDER BY CASE status WHEN 'BACKLOG' THEN 0 WHEN 'READY' THEN 1 WHEN 'WORKING' THEN 2 WHEN 'REVIEW' THEN 3 WHEN 'HUMAN_REVIEW' THEN 4 WHEN 'BLOCKED' THEN 5 WHEN 'FAILED' THEN 6 WHEN 'DONE' THEN 7 ELSE 8 END, created_at, id LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := r.database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	values := make([]task.Task, 0)
	for rows.Next() {
		value, err := scanTask(rows)
		if err != nil {
			return nil, closeRowsAfter(rows, "tasks", fmt.Errorf("scan task: %w", err))
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "tasks", fmt.Errorf("iterate tasks: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close task rows: %w", err)
	}
	return values, nil
}

func (r *TaskRepository) Update(ctx context.Context, value task.Task) error {
	result, err := r.database.ExecContext(ctx, `UPDATE tasks SET title=?, description=?, status=?, priority=?, assigned_to=NULLIF(?,''),
		human_review=?, parent_id=NULLIF(?,''), updated_at=?, last_actor_type=?, last_actor_id=?, last_reason=?, version=? WHERE id=? AND version=?`,
		value.Title(), value.Description(), value.Status(), value.Priority(), value.AssignedTo(), value.HumanReview(), value.ParentID(),
		formatDatabaseTime(value.UpdatedAt()), value.LastActorType(), value.LastActorID(), value.LastReason(), value.Version(), value.ID(), value.Version()-1)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated tasks: %w", err)
	}
	if count == 0 {
		var exists bool
		if err := r.database.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM tasks WHERE id=?)", value.ID()).Scan(&exists); err != nil {
			return fmt.Errorf("check task after update conflict: %w", err)
		}
		if !exists {
			return task.ErrNotFound
		}
		return task.ErrConflict
	}
	return nil
}

func (r *TaskRepository) ListAuditEvents(ctx context.Context, id string) ([]task.AuditEvent, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, project_id, COALESCE(task_id,''), actor_type, actor_id, event_type,
		from_status, to_status, reason, created_at FROM audit_events WHERE resource_type='task' AND resource_id=? ORDER BY sequence`, id)
	if err != nil {
		return nil, fmt.Errorf("query task audit events: %w", err)
	}
	values := make([]task.AuditEvent, 0)
	for rows.Next() {
		var value task.AuditEvent
		var created string
		if err := rows.Scan(&value.ID, &value.ProjectID, &value.TaskID, &value.ActorType, &value.ActorID, &value.EventType,
			&value.FromStatus, &value.ToStatus, &value.Reason, &created); err != nil {
			return nil, closeRowsAfter(rows, "task audit events", fmt.Errorf("scan task audit event: %w", err))
		}
		value.CreatedAt, err = parseDatabaseTime(created)
		if err != nil {
			return nil, closeRowsAfter(rows, "task audit events", fmt.Errorf("parse task audit time: %w", err))
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "task audit events", fmt.Errorf("iterate task audit events: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close task audit event rows: %w", err)
	}
	return values, nil
}

type taskRowScanner interface{ Scan(...any) error }

const taskSelect = `SELECT id, project_id, title, description, status, priority, created_by_type, created_by,
	COALESCE(assigned_to,''), human_review, COALESCE(parent_id,''), created_at, updated_at, last_actor_type, last_actor_id, last_reason, version FROM tasks`

func scanTask(row taskRowScanner) (task.Task, error) {
	var value task.Snapshot
	var createdAt, updatedAt string
	if err := row.Scan(&value.ID, &value.ProjectID, &value.Title, &value.Description, &value.Status, &value.Priority,
		&value.CreatedByType, &value.CreatedBy, &value.AssignedTo, &value.HumanReview, &value.ParentID, &createdAt, &updatedAt,
		&value.LastActorType, &value.LastActorID, &value.LastReason, &value.Version); err != nil {
		return task.Task{}, err
	}
	var err error
	value.CreatedAt, err = parseDatabaseTime(createdAt)
	if err != nil {
		return task.Task{}, err
	}
	value.UpdatedAt, err = parseDatabaseTime(updatedAt)
	if err != nil {
		return task.Task{}, err
	}
	return task.Restore(value)
}

func writeTask(ctx context.Context, executor sqlExecutor, value task.Task) error {
	snapshot := value.Snapshot()
	_, err := executor.ExecContext(ctx, `INSERT INTO tasks(id,project_id,title,description,status,priority,created_by_type,created_by,assigned_to,
		human_review,parent_id,created_at,updated_at,last_actor_type,last_actor_id,last_reason,version)
		VALUES(?,?,?,?,?,?,?, ?,NULLIF(?,''),?,NULLIF(?,''),?,?,?,?,?,?)`,
		snapshot.ID, snapshot.ProjectID, snapshot.Title, snapshot.Description, snapshot.Status, snapshot.Priority, snapshot.CreatedByType,
		snapshot.CreatedBy, snapshot.AssignedTo, snapshot.HumanReview, snapshot.ParentID, formatDatabaseTime(snapshot.CreatedAt),
		formatDatabaseTime(snapshot.UpdatedAt), snapshot.LastActorType, snapshot.LastActorID, snapshot.LastReason, snapshot.Version)
	return err
}

func formatDatabaseTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
