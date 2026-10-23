package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNotificationNotFound = errors.New("notification not found")
var ErrNotificationRetryLimit = errors.New("notification retry limit reached")

type Notification struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	EventType      string     `json:"event_type"`
	ResourceID     string     `json:"resource_id,omitempty"`
	Severity       string     `json:"severity"`
	Summary        string     `json:"summary"`
	CreatedAt      time.Time  `json:"created_at"`
	DeliveryState  string     `json:"delivery_state"`
	DeliveryTries  int        `json:"delivery_attempts"`
	DeliveryError  string     `json:"delivery_error,omitempty"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
}

type NotificationRepository struct{ database *Database }

func NewNotificationRepository(database *Database) (*NotificationRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &NotificationRepository{database: database}, nil
}

func (r *NotificationRepository) List(ctx context.Context, projectID string, limit, offset int) ([]Notification, error) {
	rows, err := r.database.QueryContext(ctx, notificationSelect+` WHERE project_id=? ORDER BY created_at DESC, id LIMIT ? OFFSET ?`, projectID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	values := make([]Notification, 0)
	for rows.Next() {
		value, err := scanNotification(rows)
		if err != nil {
			return nil, closeRowsAfter(rows, "notifications", fmt.Errorf("scan notification: %w", err))
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "notifications", fmt.Errorf("iterate notifications: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close notification rows: %w", err)
	}
	return values, nil
}

func (r *NotificationRepository) Pending(ctx context.Context, limit int) ([]Notification, error) {
	rows, err := r.database.QueryContext(ctx, notificationSelect+` WHERE delivery_state='PENDING' AND delivery_attempts < 5 ORDER BY created_at, id LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query pending notifications: %w", err)
	}
	values := make([]Notification, 0)
	for rows.Next() {
		value, err := scanNotification(rows)
		if err != nil {
			return nil, closeRowsAfter(rows, "pending notifications", fmt.Errorf("scan pending notification: %w", err))
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "pending notifications", fmt.Errorf("iterate pending notifications: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close pending notification rows: %w", err)
	}
	return values, nil
}

func (r *NotificationRepository) RecordDelivery(ctx context.Context, id string, deliveryErr error) error {
	state, message := "DELIVERED", ""
	if deliveryErr != nil {
		state, message = "PENDING", "subscriber_unavailable"
	}
	_, err := r.database.ExecContext(ctx, `UPDATE notifications SET delivery_attempts=delivery_attempts+1,
		 delivery_state=CASE WHEN ?='DELIVERED' THEN 'DELIVERED' WHEN delivery_attempts+1 >= 3 THEN 'FAILED' ELSE 'PENDING' END,
	 delivery_error=?, delivered_at=CASE WHEN ?='DELIVERED' THEN CURRENT_TIMESTAMP ELSE delivered_at END WHERE id=? AND delivery_state='PENDING' AND delivery_attempts < 5`, state, message, state, id)
	if err != nil {
		return fmt.Errorf("record notification delivery: %w", err)
	}
	return nil
}

func (r *NotificationRepository) Acknowledge(ctx context.Context, projectID, id string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin notification acknowledgement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE notifications SET acknowledged_at=COALESCE(acknowledged_at,CURRENT_TIMESTAMP), read_at=COALESCE(read_at,CURRENT_TIMESTAMP) WHERE project_id=? AND id=?`, projectID, id)
	if err != nil {
		return fmt.Errorf("acknowledge notification: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count acknowledged notifications: %w", err)
	}
	if count == 0 {
		return ErrNotificationNotFound
	}
	return recordNotificationAction(ctx, tx, projectID, id, "notification.acknowledged", "administrator_acknowledged")
}

func (r *NotificationRepository) MarkRead(ctx context.Context, projectID, id string) error {
	result, err := r.database.ExecContext(ctx, `UPDATE notifications SET read_at=COALESCE(read_at,CURRENT_TIMESTAMP) WHERE project_id=? AND id=?`, projectID, id)
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count read notifications: %w", err)
	}
	if count == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

func (r *NotificationRepository) Retry(ctx context.Context, projectID, id string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin notification retry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE notifications SET delivery_state='PENDING', delivery_error='' WHERE project_id=? AND id=? AND delivery_state='FAILED' AND delivery_attempts < 5 AND acknowledged_at IS NULL`, projectID, id)
	if err != nil {
		return fmt.Errorf("retry notification delivery: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count retried notifications: %w", err)
	}
	if count == 0 {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM notifications WHERE project_id=? AND id=?)", projectID, id).Scan(&exists); err != nil {
			return fmt.Errorf("check notification retry target: %w", err)
		}
		if !exists {
			return ErrNotificationNotFound
		}
		return ErrNotificationRetryLimit
	}
	return recordNotificationAction(ctx, tx, projectID, id, "notification.delivery.retried", "administrator_requested_delivery_retry")
}

func recordNotificationAction(ctx context.Context, tx *Transaction, projectID, id, eventType, reason string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id,actor_type,actor_id,event_type,resource_type,resource_id,reason)
		VALUES (?, 'administrator', 'local-administrator', ?, 'notification', ?, ?)`, projectID, eventType, id, reason); err != nil {
		return fmt.Errorf("record notification audit event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit notification action: %w", err)
	}
	return nil
}

const notificationSelect = `SELECT id, project_id, event_type, resource_id, severity, summary, created_at, delivery_state,
	delivery_attempts, delivery_error, delivered_at, read_at, acknowledged_at FROM notifications`

type notificationScanner interface{ Scan(...any) error }

func scanNotification(row notificationScanner) (Notification, error) {
	var value Notification
	var created string
	var delivered, read, acknowledged sql.NullString
	err := row.Scan(&value.ID, &value.ProjectID, &value.EventType, &value.ResourceID, &value.Severity, &value.Summary, &created, &value.DeliveryState, &value.DeliveryTries, &value.DeliveryError, &delivered, &read, &acknowledged)
	if err != nil {
		return value, err
	}
	value.CreatedAt, err = parseDatabaseTime(created)
	if err != nil {
		return value, err
	}
	for _, item := range []struct {
		raw    sql.NullString
		target **time.Time
	}{{delivered, &value.DeliveredAt}, {read, &value.ReadAt}, {acknowledged, &value.AcknowledgedAt}} {
		if item.raw.Valid {
			parsed, err := parseDatabaseTime(item.raw.String)
			if err != nil {
				return value, err
			}
			*item.target = &parsed
		}
	}
	return value, nil
}
