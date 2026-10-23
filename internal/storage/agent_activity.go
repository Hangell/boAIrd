package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/heartbeat"
)

// refreshInterval floors how often an open session is rewritten. MCP clients
// issue several requests per tool call and every durable write is costly on
// SQLite, so activity inside this window reuses the stored timestamp.
const refreshInterval = time.Second

// AgentActivityRepository persists authenticated Agent sessions and the
// server-derived status transitions observed from them.
type AgentActivityRepository struct{ database *Database }

// NewAgentActivityRepository builds the repository.
func NewAgentActivityRepository(database *Database) (*AgentActivityRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &AgentActivityRepository{database: database}, nil
}

// RecordActivity refreshes the open session bound to (agent, token), creating it
// on first contact or after expiry. The identity always comes from a verified
// credential.
func (r *AgentActivityRepository) RecordActivity(ctx context.Context, identity heartbeat.Identity, at time.Time) (heartbeat.Session, bool, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return heartbeat.Session{}, false, fmt.Errorf("begin agent activity: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var session heartbeat.Session
	var started, lastSeen string
	err = tx.QueryRowContext(ctx, `SELECT id, agent_id, token_id, started_at, last_seen_at
		FROM agent_sessions WHERE agent_id = ? AND token_id = ? AND ended_at IS NULL`, identity.AgentID, identity.TokenID).
		Scan(&session.ID, &session.AgentID, &session.TokenID, &started, &lastSeen)
	switch {
	case err == nil:
		startedAt, err := parseDatabaseTime(started)
		if err != nil {
			return heartbeat.Session{}, false, fmt.Errorf("parse agent session start: %w", err)
		}
		lastSeenAt, err := parseDatabaseTime(lastSeen)
		if err != nil {
			return heartbeat.Session{}, false, fmt.Errorf("parse agent session activity: %w", err)
		}
		session.StartedAt = startedAt
		session.LastSeenAt = lastSeenAt
		if at.Sub(lastSeenAt) < refreshInterval {
			return session, false, commitAgentActivity(tx)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE agent_sessions SET last_seen_at = ? WHERE id = ?`, formatDatabaseTime(at), session.ID); err != nil {
			return heartbeat.Session{}, false, fmt.Errorf("refresh agent session: %w", err)
		}
		session.LastSeenAt = at.UTC()
	case errors.Is(err, sql.ErrNoRows):
		id, err := newSessionID()
		if err != nil {
			return heartbeat.Session{}, false, err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM agents WHERE id = ?)", identity.AgentID).Scan(&exists); err != nil {
			return heartbeat.Session{}, false, fmt.Errorf("check agent for session: %w", err)
		}
		if !exists {
			return heartbeat.Session{}, false, agent.ErrNotFound
		}
		session = heartbeat.Session{ID: id, AgentID: identity.AgentID, TokenID: identity.TokenID, StartedAt: at.UTC(), LastSeenAt: at.UTC()}
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_sessions(id, agent_id, token_id, started_at, last_seen_at)
			VALUES (?, ?, ?, ?, ?)`, session.ID, session.AgentID, session.TokenID, formatDatabaseTime(at), formatDatabaseTime(at)); err != nil {
			return heartbeat.Session{}, false, fmt.Errorf("open agent session: %w", err)
		}
		return session, true, commitAgentActivity(tx)
	default:
		return heartbeat.Session{}, false, fmt.Errorf("read agent session: %w", err)
	}
	return session, false, commitAgentActivity(tx)
}

func commitAgentActivity(tx *Transaction) error {
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent activity: %w", err)
	}
	return nil
}

// Activity returns the derivation inputs for the requested Agents.
func (r *AgentActivityRepository) Activity(ctx context.Context, agentIDs []string) (map[string]heartbeat.Activity, error) {
	result := make(map[string]heartbeat.Activity, len(agentIDs))
	if len(agentIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(agentIDs)), ",")
	args := make([]any, len(agentIDs))
	for i, id := range agentIDs {
		args[i] = id
		result[id] = heartbeat.Activity{}
	}

	rows, err := r.database.QueryContext(ctx, `SELECT s.agent_id, COUNT(*),
		(SELECT o.id FROM agent_sessions o WHERE o.agent_id = s.agent_id AND o.ended_at IS NULL ORDER BY o.last_seen_at DESC, o.id LIMIT 1),
		(SELECT o.last_seen_at FROM agent_sessions o WHERE o.agent_id = s.agent_id AND o.ended_at IS NULL ORDER BY o.last_seen_at DESC, o.id LIMIT 1)
		FROM agent_sessions s WHERE s.agent_id IN (`+placeholders+`) GROUP BY s.agent_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("query agent sessions: %w", err)
	}
	for rows.Next() {
		var agentID string
		var sessions int
		var sessionID, lastSeen sql.NullString
		if err := rows.Scan(&agentID, &sessions, &sessionID, &lastSeen); err != nil {
			return nil, closeRowsAfter(rows, "agent sessions", fmt.Errorf("scan agent session: %w", err))
		}
		activity := result[agentID]
		activity.Sessions = sessions
		if sessionID.Valid {
			activity.SessionID = sessionID.String
		}
		if lastSeen.Valid {
			parsed, err := parseDatabaseTime(lastSeen.String)
			if err != nil {
				return nil, closeRowsAfter(rows, "agent sessions", fmt.Errorf("parse agent last seen: %w", err))
			}
			activity.LastSeenAt = &parsed
		}
		result[agentID] = activity
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "agent sessions", fmt.Errorf("iterate agent sessions: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close agent session rows: %w", err)
	}

	rows, err = r.database.QueryContext(ctx, `SELECT assigned_to,
		SUM(CASE WHEN status = 'WORKING' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status = 'REVIEW' THEN 1 ELSE 0 END),
		MAX(CASE WHEN status = 'FAILED' THEN updated_at END)
		FROM tasks WHERE assigned_to IN (`+placeholders+`) GROUP BY assigned_to`, args...)
	if err != nil {
		return nil, fmt.Errorf("query agent work: %w", err)
	}
	for rows.Next() {
		var agentID string
		var working, review int
		var failedAt sql.NullString
		if err := rows.Scan(&agentID, &working, &review, &failedAt); err != nil {
			return nil, closeRowsAfter(rows, "agent work", fmt.Errorf("scan agent work: %w", err))
		}
		activity := result[agentID]
		activity.Working = working
		activity.Review = review
		if failedAt.Valid {
			parsed, err := parseDatabaseTime(failedAt.String)
			if err != nil {
				return nil, closeRowsAfter(rows, "agent work", fmt.Errorf("parse agent failure time: %w", err))
			}
			activity.FailedAt = &parsed
		}
		result[agentID] = activity
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "agent work", fmt.Errorf("iterate agent work: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close agent work rows: %w", err)
	}

	rows, err = r.database.QueryContext(ctx, `SELECT id, status FROM agents WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("query agent status: %w", err)
	}
	for rows.Next() {
		var agentID, stored string
		if err := rows.Scan(&agentID, &stored); err != nil {
			return nil, closeRowsAfter(rows, "agent status", fmt.Errorf("scan agent status: %w", err))
		}
		activity := result[agentID]
		activity.StoredStatus = agent.Status(stored)
		result[agentID] = activity
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "agent status", fmt.Errorf("iterate agent status: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close agent status rows: %w", err)
	}
	return result, nil
}

// AgentsWithSessions lists the Agents that need reconciliation.
func (r *AgentActivityRepository) AgentsWithSessions(ctx context.Context) ([]string, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT agent_id FROM agent_sessions
		UNION SELECT id FROM agents WHERE status <> 'IDLE' ORDER BY agent_id`)
	if err != nil {
		return nil, fmt.Errorf("query agents with sessions: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, closeRowsAfter(rows, "agents with sessions", fmt.Errorf("scan agent with session: %w", err))
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "agents with sessions", fmt.Errorf("iterate agents with sessions: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close agents with sessions rows: %w", err)
	}
	return ids, nil
}

// CloseSessionsBefore ends every open session whose last activity is at or
// before the cutoff and returns the sessions that were closed.
func (r *AgentActivityRepository) CloseSessionsBefore(ctx context.Context, cutoff, at time.Time) ([]heartbeat.Session, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin session expiry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id, agent_id, token_id, started_at, last_seen_at FROM agent_sessions
		WHERE ended_at IS NULL AND last_seen_at <= ? ORDER BY last_seen_at, id`, formatDatabaseTime(cutoff))
	if err != nil {
		return nil, fmt.Errorf("query expired sessions: %w", err)
	}
	closed := make([]heartbeat.Session, 0)
	for rows.Next() {
		var session heartbeat.Session
		var started, lastSeen string
		if err := rows.Scan(&session.ID, &session.AgentID, &session.TokenID, &started, &lastSeen); err != nil {
			return nil, closeRowsAfter(rows, "expired sessions", fmt.Errorf("scan expired session: %w", err))
		}
		startedAt, err := parseDatabaseTime(started)
		if err != nil {
			return nil, closeRowsAfter(rows, "expired sessions", fmt.Errorf("parse expired session start: %w", err))
		}
		lastSeenAt, err := parseDatabaseTime(lastSeen)
		if err != nil {
			return nil, closeRowsAfter(rows, "expired sessions", fmt.Errorf("parse expired session activity: %w", err))
		}
		session.StartedAt, session.LastSeenAt = startedAt, lastSeenAt
		closed = append(closed, session)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "expired sessions", fmt.Errorf("iterate expired sessions: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close expired session rows: %w", err)
	}
	for _, session := range closed {
		if _, err := tx.ExecContext(ctx, `UPDATE agent_sessions SET ended_at = ? WHERE id = ? AND ended_at IS NULL`, formatDatabaseTime(at), session.ID); err != nil {
			return nil, fmt.Errorf("expire agent session: %w", err)
		}
		endedAt := at.UTC()
		session.EndedAt = &endedAt
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit session expiry: %w", err)
	}
	return closed, nil
}

// ApplyStatus persists a derived status transition and its immutable event.
// It reports the stored status as the event source and whether anything changed.
func (r *AgentActivityRepository) ApplyStatus(ctx context.Context, change heartbeat.StatusChange) (heartbeat.StatusEvent, bool, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return heartbeat.StatusEvent{}, false, fmt.Errorf("begin agent status change: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var stored string
	if err := tx.QueryRowContext(ctx, "SELECT status FROM agents WHERE id = ?", change.AgentID).Scan(&stored); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return heartbeat.StatusEvent{}, false, agent.ErrNotFound
		}
		return heartbeat.StatusEvent{}, false, fmt.Errorf("read agent status: %w", err)
	}
	if agent.Status(stored) == change.To {
		return heartbeat.StatusEvent{}, false, nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE agents SET status = ? WHERE id = ?", string(change.To), change.AgentID); err != nil {
		return heartbeat.StatusEvent{}, false, fmt.Errorf("update agent status: %w", err)
	}
	var sequence int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO agent_status_events(agent_id, session_id, from_status, to_status, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING sequence`, change.AgentID, change.SessionID, stored, string(change.To), change.Reason, formatDatabaseTime(change.At)).Scan(&sequence); err != nil {
		return heartbeat.StatusEvent{}, false, fmt.Errorf("record agent status event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return heartbeat.StatusEvent{}, false, fmt.Errorf("commit agent status change: %w", err)
	}
	return heartbeat.StatusEvent{
		Sequence: sequence, AgentID: change.AgentID, SessionID: change.SessionID,
		From: agent.Status(stored), To: change.To, Reason: change.Reason, CreatedAt: change.At.UTC(),
	}, true, nil
}

// StatusEvents returns the most recent status transitions for one Agent.
func (r *AgentActivityRepository) StatusEvents(ctx context.Context, agentID string, limit int) ([]heartbeat.StatusEvent, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT sequence, agent_id, session_id, from_status, to_status, reason, created_at
		FROM agent_status_events WHERE agent_id = ? ORDER BY sequence DESC LIMIT ?`, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("query agent status events: %w", err)
	}
	events := make([]heartbeat.StatusEvent, 0)
	for rows.Next() {
		var event heartbeat.StatusEvent
		var from, to, created string
		if err := rows.Scan(&event.Sequence, &event.AgentID, &event.SessionID, &from, &to, &event.Reason, &created); err != nil {
			return nil, closeRowsAfter(rows, "agent status events", fmt.Errorf("scan agent status event: %w", err))
		}
		event.From, event.To = agent.Status(from), agent.Status(to)
		parsed, err := parseDatabaseTime(created)
		if err != nil {
			return nil, closeRowsAfter(rows, "agent status events", fmt.Errorf("parse agent status event time: %w", err))
		}
		event.CreatedAt = parsed
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsAfter(rows, "agent status events", fmt.Errorf("iterate agent status events: %w", err))
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close agent status event rows: %w", err)
	}
	return events, nil
}

func newSessionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate agent session ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
