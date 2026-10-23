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
	database *sql.DB
}

func NewUserRepository(database *sql.DB) (*UserRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &UserRepository{database: database}, nil
}

func (r *UserRepository) Create(ctx context.Context, value user.User) error {
	if _, err := r.database.ExecContext(ctx, `INSERT INTO users(id, name, role) VALUES (?, ?, ?)`,
		value.ID(), value.Name(), string(value.Role())); err != nil {
		if isUniqueConstraint(err) {
			return user.ErrAlreadyExists
		}
		return fmt.Errorf("insert user: %w", err)
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

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
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
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete user: %w", err)
	}
	return nil
}

func (r *UserRepository) SetProjectMembership(ctx context.Context, projectID, userID string, role user.MembershipRole) error {
	if _, err := r.database.ExecContext(ctx, `INSERT INTO project_members(project_id, user_id, role) VALUES (?, ?, ?)
		ON CONFLICT(project_id, user_id) DO UPDATE SET role = excluded.role`, projectID, userID, string(role)); err != nil {
		return fmt.Errorf("set project membership: %w", err)
	}
	return nil
}

func (r *UserRepository) RemoveProjectMembership(ctx context.Context, projectID, userID string) error {
	if _, err := r.database.ExecContext(ctx, `DELETE FROM project_members WHERE project_id = ? AND user_id = ?`, projectID, userID); err != nil {
		return fmt.Errorf("remove project membership: %w", err)
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

func scanUser(row rowScanner) (user.User, error) {
	var id, name, role string
	if err := row.Scan(&id, &name, &role); err != nil {
		return user.User{}, err
	}
	return user.Restore(id, name, user.Role(role))
}

func isUniqueConstraint(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
