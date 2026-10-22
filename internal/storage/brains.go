package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"boAIrd/internal/brain"
)

var ErrBrainAlreadyConfigured = brain.ErrAlreadyExists

type BrainRepository struct{ database *sql.DB }

func NewBrainRepository(database *sql.DB) (*BrainRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &BrainRepository{database: database}, nil
}

func (r *BrainRepository) Create(ctx context.Context, value brain.Brain) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Brain creation: %w", err)
	}
	defer tx.Rollback()
	var status string
	var current sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT status, brain_id FROM projects WHERE id = ?", value.ProjectID()).Scan(&status, &current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return brain.ErrProjectNotFound
		}
		return fmt.Errorf("read Brain project: %w", err)
	}
	if status == "ARCHIVED" {
		return brain.ErrProjectUnavailable
	}
	if current.Valid {
		return ErrBrainAlreadyConfigured
	}
	var validAgent bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agents a JOIN agent_capabilities c ON c.agent_id=a.id
		WHERE a.id=? AND a.runtime='API' AND a.credential_id IS NOT NULL AND c.capability='brain.execute')`, value.AgentID()).Scan(&validAgent)
	if err != nil {
		return fmt.Errorf("validate Brain agent: %w", err)
	}
	if !validAgent {
		return brain.ErrInvalidAgent
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO brains(id, project_id, active) VALUES (?, ?, 1)", value.ID(), value.ProjectID()); err != nil {
		return fmt.Errorf("insert Brain: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO brain_configuration(brain_id, agent_id, autonomy) VALUES (?, ?, ?)", value.ID(), value.AgentID(), value.Autonomy()); err != nil {
		return fmt.Errorf("insert Brain configuration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE projects SET brain_id=?, updated_at=CURRENT_TIMESTAMP WHERE id=? AND brain_id IS NULL AND status <> 'ARCHIVED'", value.ID(), value.ProjectID()); err != nil {
		return fmt.Errorf("attach Brain to project: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Brain creation: %w", err)
	}
	return nil
}

func (r *BrainRepository) GetActiveByProject(ctx context.Context, projectID string) (brain.Brain, error) {
	var id, agentID string
	var autonomy brain.AutonomyMode
	err := r.database.QueryRowContext(ctx, `SELECT b.id, bc.agent_id, bc.autonomy FROM brains b
		JOIN brain_configuration bc ON bc.brain_id=b.id JOIN projects p ON p.brain_id=b.id
		WHERE b.project_id=? AND b.active=1`, projectID).Scan(&id, &agentID, &autonomy)
	if errors.Is(err, sql.ErrNoRows) {
		return brain.Brain{}, brain.ErrNotFound
	}
	if err != nil {
		return brain.Brain{}, fmt.Errorf("read active Brain: %w", err)
	}
	return brain.New(id, projectID, agentID, autonomy)
}
