package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"boAIrd/internal/brain"
	"boAIrd/internal/project"
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
	defer func() { _ = tx.Rollback() }()
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
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, to_status, reason)
		VALUES (?, 'administrator', ?, 'brain.attached', 'brain', ?, ?, 'project Brain configured')`, value.ProjectID(), project.LocalAdministratorID, value.ID(), value.AgentID()); err != nil {
		return fmt.Errorf("audit Brain attachment: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Brain creation: %w", err)
	}
	return nil
}

func (r *BrainRepository) Replace(ctx context.Context, current, replacement brain.Brain, actorID, reason string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Brain replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var projectStatus string
	var currentAgentID string
	var currentAutonomy string
	if err := tx.QueryRowContext(ctx, `SELECT p.status, bc.agent_id, bc.autonomy FROM projects p
		JOIN brains b ON b.project_id=p.id AND b.id=p.brain_id AND b.active=1
		JOIN brain_configuration bc ON bc.brain_id=b.id WHERE p.id=? AND b.id=?`, current.ProjectID(), current.ID()).
		Scan(&projectStatus, &currentAgentID, &currentAutonomy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return brain.ErrNotFound
		}
		return fmt.Errorf("read current Brain configuration: %w", err)
	}
	if projectStatus == "ARCHIVED" {
		return brain.ErrProjectUnavailable
	}
	if currentAgentID != current.AgentID() || currentAutonomy != string(current.Autonomy()) {
		return brain.ErrConfigurationChanged
	}
	var validAgent bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agents a JOIN agent_capabilities c ON c.agent_id=a.id
		WHERE a.id=? AND a.runtime='API' AND a.credential_id IS NOT NULL AND c.capability='brain.execute')`, replacement.AgentID()).Scan(&validAgent); err != nil {
		return fmt.Errorf("validate replacement Brain agent: %w", err)
	}
	if !validAgent {
		return brain.ErrInvalidAgent
	}
	result, err := tx.ExecContext(ctx, `UPDATE brain_configuration SET agent_id=?, autonomy=?
		WHERE brain_id=? AND agent_id=? AND autonomy=?`, replacement.AgentID(), replacement.Autonomy(), current.ID(), current.AgentID(), current.Autonomy())
	if err != nil {
		return fmt.Errorf("update Brain configuration: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check Brain replacement update: %w", err)
	}
	if rows != 1 {
		return brain.ErrConfigurationChanged
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, from_status, to_status, reason)
		VALUES (?, 'user', ?, 'brain.replaced', 'brain', ?, ?, ?, ?)`, current.ProjectID(), actorID, current.ID(),
		current.AgentID()+"/"+string(current.Autonomy()), replacement.AgentID()+"/"+string(replacement.Autonomy()), reason); err != nil {
		return fmt.Errorf("audit Brain replacement: %w", err)
	}
	if current.Autonomy() != replacement.Autonomy() {
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(project_id, actor_type, actor_id, event_type, resource_type, resource_id, from_status, to_status, reason)
			VALUES (?, 'user', ?, 'brain.autonomy.changed', 'brain', ?, ?, ?, ?)`, current.ProjectID(), actorID, current.ID(),
			string(current.Autonomy()), string(replacement.Autonomy()), reason); err != nil {
			return fmt.Errorf("audit Brain autonomy change: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Brain replacement: %w", err)
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
