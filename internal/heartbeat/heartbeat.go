// Package heartbeat derives Agent runtime status from authenticated sessions
// and observed work. Status is server-owned: clients authenticate, they never
// choose a status.
package heartbeat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"boAIrd/internal/agent"
)

const (
	// DefaultStaleAfter is how long an Agent may stay silent before its session
	// is considered stale.
	DefaultStaleAfter = 90 * time.Second
	// DefaultOfflineAfter is how long an Agent may stay silent before its
	// session expires and the Agent is considered offline.
	DefaultOfflineAfter = 5 * time.Minute
	// DefaultStatusInterval is how often transitions are reconciled in the background.
	DefaultStatusInterval = 30 * time.Second

	maxInterval = 24 * time.Hour
	maxEvents   = 100
	defaultPage = 50
)

// Status-event reasons. They explain why the server observed a transition.
const (
	ReasonSessionOpened  = "session.opened"
	ReasonSessionResumed = "session.resumed"
	ReasonSessionExpired = "session.expired"
	ReasonWorkChanged    = "work.changed"
	ReasonReconciled     = "status.reconciled"
)

var (
	ErrInvalidInput  = errors.New("invalid heartbeat input")
	ErrInvalidPolicy = errors.New("invalid heartbeat policy")
)

// Identity is the authenticated principal behind a heartbeat. It always comes
// from a verified credential, never from request payload or an Agent name.
type Identity struct {
	AgentID string
	TokenID string
}

// Policy holds the liveness thresholds applied to every Agent.
type Policy struct {
	StaleAfter   time.Duration
	OfflineAfter time.Duration
	ErrorWindow  time.Duration
}

// NewPolicy validates liveness thresholds and derives the error window.
func NewPolicy(staleAfter, offlineAfter time.Duration) (Policy, error) {
	if staleAfter < time.Second || staleAfter >= offlineAfter || offlineAfter > maxInterval {
		return Policy{}, fmt.Errorf("%w: require 1s <= stale < offline <= %s", ErrInvalidPolicy, maxInterval)
	}
	return Policy{StaleAfter: staleAfter, OfflineAfter: offlineAfter, ErrorWindow: staleAfter}, nil
}

// Activity is the observed input for status derivation.
type Activity struct {
	// Sessions is the number of sessions ever recorded for the Agent.
	Sessions int
	// SessionID identifies the most recent open session, if any.
	SessionID string
	// LastSeenAt is the most recent authenticated activity of an open session.
	LastSeenAt *time.Time
	// Working and Review count assigned tasks in that state.
	Working int
	Review  int
	// FailedAt is when an assigned task most recently entered FAILED.
	FailedAt *time.Time
	// StoredStatus is the last persisted status, used to explain transitions.
	StoredStatus agent.Status
}

// Session is one authenticated runtime connection. A session is bound to the
// Agent identity and to the credential that opened it.
type Session struct {
	ID         string
	AgentID    string
	TokenID    string
	StartedAt  time.Time
	LastSeenAt time.Time
	EndedAt    *time.Time
}

// StatusEvent is the immutable record of one observed status transition.
type StatusEvent = agent.StatusEvent

// StatusChange asks the repository to persist a derived status.
type StatusChange struct {
	AgentID   string
	SessionID string
	To        agent.Status
	Reason    string
	At        time.Time
}

// Repository persists sessions, derived status transitions, and events.
type Repository interface {
	RecordActivity(ctx context.Context, identity Identity, at time.Time) (session Session, created bool, err error)
	Activity(ctx context.Context, agentIDs []string) (map[string]Activity, error)
	AgentsWithSessions(ctx context.Context) ([]string, error)
	CloseSessionsBefore(ctx context.Context, cutoff, at time.Time) ([]Session, error)
	ApplyStatus(ctx context.Context, change StatusChange) (StatusEvent, bool, error)
	StatusEvents(ctx context.Context, agentID string, limit int) ([]StatusEvent, error)
}

// Derive computes the server-owned status. Precedence is deliberate:
//
//	no session yet                -> IDLE
//	silent >= offline threshold   -> OFFLINE
//	silent >= stale threshold     -> STALE
//	assigned task in WORKING      -> BUSY
//	assigned task in REVIEW       -> WAITING
//	recent FAILED task            -> ERROR
//	fresh session without work    -> ONLINE
//
// Liveness outranks work: an Agent that stops reporting is STALE or OFFLINE
// even if the board still shows it holding a task.
func Derive(now time.Time, policy Policy, activity Activity) agent.Observed {
	observed := agent.Observed{Status: agent.StatusIdle, LastSeenAt: activity.LastSeenAt}
	if activity.LastSeenAt == nil || activity.SessionID == "" {
		if activity.Sessions == 0 {
			return observed
		}
		observed.Status = agent.StatusOffline
		return observed
	}
	observed.SessionOpen = true
	silent := now.Sub(*activity.LastSeenAt)
	switch {
	case silent >= policy.OfflineAfter:
		observed.Status = agent.StatusOffline
	case silent >= policy.StaleAfter:
		observed.Status = agent.StatusStale
	case activity.Working > 0:
		observed.Status = agent.StatusBusy
	case activity.Review > 0:
		observed.Status = agent.StatusWaiting
	case activity.FailedAt != nil && now.Sub(*activity.FailedAt) < policy.ErrorWindow:
		observed.Status = agent.StatusError
	default:
		observed.Status = agent.StatusOnline
	}
	return observed
}

// SessionReason explains a newly created session: a reconnect after expiry is
// a resume, the first session is an open.
func SessionReason(sessions int) string {
	if sessions > 1 {
		return ReasonSessionResumed
	}
	return ReasonSessionOpened
}

func validIdentity(identity Identity) bool {
	for _, value := range []string{identity.AgentID, identity.TokenID} {
		if value == "" || strings.TrimSpace(value) != value || len(value) > 200 {
			return false
		}
	}
	return true
}
