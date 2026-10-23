package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"boAIrd/internal/user"
)

// UserRepository persists authenticated users and project membership.
type UserRepository struct {
	database *Database
}

func NewUserRepository(database *Database) (*UserRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &UserRepository{database: database}, nil
}

func (r *UserRepository) Create(ctx context.Context, actorID string, value user.User) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO users(id, name, role) VALUES (?, ?, ?)`,
		value.ID(), value.Name(), string(value.Role())); err != nil {
		if isUniqueConstraint(err) || strings.Contains(err.Error(), "user identity was retired") {
			return user.ErrAlreadyExists
		}
		return fmt.Errorf("insert user: %w", err)
	}
	if err := recordUserAuthorityEvent(ctx, tx, actorID, value.ID(), "", "user.created", "", string(value.Role()), "user created"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create user: %w", err)
	}
	return nil
}

func (r *UserRepository) List(ctx context.Context) ([]user.User, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, name, role FROM users ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	values := make([]user.User, 0)
	for rows.Next() {
		value, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return values, nil
}

func (r *UserRepository) Get(ctx context.Context, id string) (user.User, error) {
	value, err := scanUser(r.database.QueryRowContext(ctx, `SELECT id, name, role FROM users WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return user.User{}, user.ErrNotFound
	}
	if err != nil {
		return user.User{}, fmt.Errorf("get user: %w", err)
	}
	return value, nil
}

func (r *UserRepository) Delete(ctx context.Context, actorID, id string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockUserAuthorityRow(ctx, tx, id); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return user.ErrNotFound
		}
		return fmt.Errorf("lock user before delete: %w", err)
	}
	var previousRole string
	if err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, id).Scan(&previousRole); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user.ErrNotFound
		}
		return fmt.Errorf("read user before delete: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_identity_tombstones(user_id) VALUES (?)`, id); err != nil {
		return fmt.Errorf("retire user identity: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_members WHERE user_id = ?`, id); err != nil {
		return fmt.Errorf("delete user memberships: %w", err)
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted user: %w", err)
	}
	if affected == 0 {
		return user.ErrNotFound
	}
	if err := recordUserAuthorityEvent(ctx, tx, actorID, id, "", "user.deleted", previousRole, "", "user deleted"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete user: %w", err)
	}
	return nil
}

func (r *UserRepository) SetProjectMembership(ctx context.Context, actorID, projectID, userID string, role user.MembershipRole) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set project membership: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockUserAuthorityRow(ctx, tx, userID); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return user.ErrNotFound
		}
		return fmt.Errorf("lock user before setting project membership: %w", err)
	}
	var previousRole string
	previousErr := tx.QueryRowContext(ctx, `SELECT role FROM project_members WHERE project_id = ? AND user_id = ?`, projectID, userID).Scan(&previousRole)
	if previousErr != nil && !errors.Is(previousErr, sql.ErrNoRows) {
		return fmt.Errorf("read previous project membership: %w", previousErr)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_members(project_id, user_id, role) VALUES (?, ?, ?)
		ON CONFLICT(project_id, user_id) DO UPDATE SET role = excluded.role`, projectID, userID, string(role)); err != nil {
		return fmt.Errorf("set project membership: %w", err)
	}
	action := "user.project.granted"
	if previousErr == nil {
		action = "user.project.updated"
	}
	if err := recordUserAuthorityEvent(ctx, tx, actorID, userID, projectID, action, previousRole, string(role), "project membership changed"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit project membership: %w", err)
	}
	return nil
}

func (r *UserRepository) RemoveProjectMembership(ctx context.Context, actorID, projectID, userID string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remove project membership: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockUserAuthorityRow(ctx, tx, userID); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return tx.Commit()
		}
		return fmt.Errorf("lock user before removing project membership: %w", err)
	}
	var previousRole string
	if err := tx.QueryRowContext(ctx, `SELECT role FROM project_members WHERE project_id = ? AND user_id = ?`, projectID, userID).Scan(&previousRole); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tx.Commit()
		}
		return fmt.Errorf("read previous project membership: %w", err)
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM project_members WHERE project_id = ? AND user_id = ?`, projectID, userID)
	if err != nil {
		return fmt.Errorf("remove project membership: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count removed project membership: %w", err)
	}
	if deleted == 0 {
		return tx.Commit()
	}
	if err := recordUserAuthorityEvent(ctx, tx, actorID, userID, projectID, "user.project.revoked", previousRole, "", "project membership revoked"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit project membership removal: %w", err)
	}
	return nil
}

// lockUserAuthorityRow serializes authority changes for one user on PostgreSQL
// so the previous role in each committed audit event reflects the prior commit.
func (r *UserRepository) lockUserAuthorityRow(ctx context.Context, tx *Transaction, userID string) error {
	query := `SELECT id FROM users WHERE id = ?`
	if r.database.dialect == dialectPostgres {
		query += ` FOR UPDATE`
	}
	var lockedID string
	if err := tx.QueryRowContext(ctx, query, userID).Scan(&lockedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user.ErrNotFound
		}
		return err
	}
	return nil
}

// RecordSessionIssued stores session issuance metadata without recording the
// opaque session identifier or cookie value.
func (r *UserRepository) RecordSessionIssued(ctx context.Context, actorID, userID string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session audit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, identityID := range []string{actorID, userID} {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)`, identityID).Scan(&exists); err != nil {
			return fmt.Errorf("validate session identity: %w", err)
		}
		if !exists {
			return user.ErrNotFound
		}
	}
	if err := recordUserAuthorityEvent(ctx, tx, actorID, userID, "", "user.session.issued", "", "", "administrator-issued opaque session"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session audit: %w", err)
	}
	return nil
}

func (r *UserRepository) ProjectMemberships(ctx context.Context, userID string) (map[string]user.MembershipRole, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT project_id, role FROM project_members WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("list project memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()
	values := make(map[string]user.MembershipRole)
	for rows.Next() {
		var projectID, role string
		if err := rows.Scan(&projectID, &role); err != nil {
			return nil, fmt.Errorf("scan project membership: %w", err)
		}
		values[projectID] = user.MembershipRole(role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project memberships: %w", err)
	}
	return values, nil
}

// IsUserProjectMember reports whether a user holds membership in a project.
func (r *UserRepository) IsUserProjectMember(ctx context.Context, projectID, userID string) (bool, error) {
	var member bool
	err := r.database.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id = ? AND user_id = ?)`, projectID, userID).Scan(&member)
	if err != nil {
		return false, fmt.Errorf("check project membership: %w", err)
	}
	return member, nil
}

type userAuthorityAuditExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func recordUserAuthorityEvent(ctx context.Context, executor userAuthorityAuditExecutor, actorID, userID, projectID, action, previousRole, nextRole, reason string) error {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(userID) == "" || strings.TrimSpace(action) == "" {
		return errors.New("user authority audit actor, target, and action are required")
	}
	if len(reason) > 500 {
		reason = reason[:500]
	}
	if _, err := executor.ExecContext(ctx, `INSERT INTO user_authority_audit_events
		(actor_id, event_type, target_user_id, project_id, previous_role, next_role, result, reason)
		VALUES (?, ?, ?, ?, ?, ?, 'success', ?)`, actorID, action, userID, projectID, previousRole, nextRole, reason); err != nil {
		return fmt.Errorf("record user authority audit event: %w", err)
	}
	return nil
}

func scanUser(row rowScanner) (user.User, error) {
	var id, name, role string
	if err := row.Scan(&id, &name, &role); err != nil {
		return user.User{}, err
	}
	return user.Restore(id, name, user.Role(role))
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	if state, ok := err.(interface{ SQLState() string }); ok && state.SQLState() == "23505" {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "user identity was retired")
}
