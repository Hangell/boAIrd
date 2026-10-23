package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"boAIrd/internal/project"
)

type ProjectRepository struct {
	database *Database
}

func NewProjectRepository(database *Database) (*ProjectRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &ProjectRepository{database: database}, nil
}

func (r *ProjectRepository) Create(ctx context.Context, value project.Project) error {
	snapshot := value.Snapshot()
	_, err := r.database.ExecContext(ctx, `INSERT INTO projects (
		id, name, description, status, brain_id, workspace_path, repository_url,
		human_review_required
	) VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?)`,
		snapshot.ID, snapshot.Name, snapshot.Description, snapshot.Status, snapshot.BrainID,
		snapshot.WorkspacePath, snapshot.RepositoryURL, snapshot.HumanReviewRequired)
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	return nil
}

func (r *ProjectRepository) Get(ctx context.Context, id string) (project.Project, error) {
	value, err := scanProject(r.database.QueryRowContext(ctx, projectSelect+" WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return project.Project{}, project.ErrNotFound
	}
	if err != nil {
		return project.Project{}, fmt.Errorf("get project: %w", err)
	}
	return value, nil
}

func (r *ProjectRepository) List(ctx context.Context, limit, offset int) ([]project.Project, error) {
	rows, err := r.database.QueryContext(ctx, projectSelect+" ORDER BY lower(name), id LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	projects := make([]project.Project, 0)
	for rows.Next() {
		value, err := scanProject(rows)
		if err != nil {
			return nil, closeRowsAfter(rows, "projects", fmt.Errorf("scan project: %w", err))
		}
		projects = append(projects, value)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "projects", fmt.Errorf("iterate projects: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close project rows: %w", err)
	}
	return projects, nil
}

func (r *ProjectRepository) Update(ctx context.Context, value project.Project) error {
	snapshot := value.Snapshot()
	result, err := r.database.ExecContext(ctx, `UPDATE projects SET
		name = ?, description = ?, status = ?, brain_id = NULLIF(?, ''), workspace_path = ?,
		repository_url = ?, human_review_required = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		snapshot.Name, snapshot.Description, snapshot.Status, snapshot.BrainID, snapshot.WorkspacePath,
		snapshot.RepositoryURL, snapshot.HumanReviewRequired, snapshot.ID)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated projects: %w", err)
	}
	if count == 0 {
		return project.ErrNotFound
	}
	return nil
}

func (r *ProjectRepository) HasActiveBrain(ctx context.Context, projectID, brainID string) (bool, error) {
	var active bool
	err := r.database.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM brains WHERE project_id = ? AND id = ? AND active = 1
	)`, projectID, brainID).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("check active project Brain: %w", err)
	}
	return active, nil
}

func (r *ProjectRepository) IsMember(ctx context.Context, projectID, userID string) (bool, error) {
	var member bool
	err := r.database.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id = ? AND user_id = ?)", projectID, userID).Scan(&member)
	if err != nil {
		return false, fmt.Errorf("check project membership: %w", err)
	}
	return member, nil
}

const projectSelect = `SELECT id, name, description, status, COALESCE(brain_id, ''),
	workspace_path, repository_url, human_review_required FROM projects`

type rowScanner interface {
	Scan(...any) error
}

func scanProject(row rowScanner) (project.Project, error) {
	var snapshot project.Snapshot
	if err := row.Scan(
		&snapshot.ID, &snapshot.Name, &snapshot.Description, &snapshot.Status, &snapshot.BrainID,
		&snapshot.WorkspacePath, &snapshot.RepositoryURL, &snapshot.HumanReviewRequired,
	); err != nil {
		return project.Project{}, err
	}
	value, err := project.Restore(snapshot)
	if err != nil {
		return project.Project{}, fmt.Errorf("restore project: %w", err)
	}
	return value, nil
}
