package heartbeat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"boAIrd/internal/agent"
)

// Service records authenticated activity and derives Agent status.
type Service struct {
	repository Repository
	policy     Policy
	now        func() time.Time
}

// Option customizes a Service.
type Option func(*Service)

// WithClock replaces the time source, used by tests to cross thresholds deterministically.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// NewService builds the heartbeat service.
func NewService(repository Repository, policy Policy, options ...Option) (*Service, error) {
	if repository == nil {
		return nil, errors.New("heartbeat repository is required")
	}
	if policy.StaleAfter <= 0 || policy.OfflineAfter <= policy.StaleAfter || policy.ErrorWindow <= 0 {
		return nil, fmt.Errorf("%w: thresholds must be positive and ordered", ErrInvalidPolicy)
	}
	service := &Service{repository: repository, policy: policy, now: time.Now}
	for _, option := range options {
		option(service)
	}
	return service, nil
}

// Heartbeat records authenticated activity for one identity and returns the
// derived status. The identity must come from a verified credential.
func (s *Service) Heartbeat(ctx context.Context, identity Identity) (agent.Observed, error) {
	if !validIdentity(identity) {
		return agent.Observed{}, ErrInvalidInput
	}
	now := s.now().UTC()
	_, created, err := s.repository.RecordActivity(ctx, identity, now)
	if err != nil {
		return agent.Observed{}, fmt.Errorf("record agent activity: %w", err)
	}
	activity, err := s.activityFor(ctx, identity.AgentID)
	if err != nil {
		return agent.Observed{}, err
	}
	observed := Derive(now, s.policy, activity)
	reason := ReasonWorkChanged
	if created {
		reason = SessionReason(activity.Sessions)
	} else if observed.SessionOpen && (activity.StoredStatus == agent.StatusStale || activity.StoredStatus == agent.StatusOffline) {
		reason = ReasonSessionResumed
	}
	if _, _, err := s.repository.ApplyStatus(ctx, StatusChange{
		AgentID:   identity.AgentID,
		SessionID: activity.SessionID,
		To:        observed.Status,
		Reason:    reason,
		At:        now,
	}); err != nil {
		return agent.Observed{}, fmt.Errorf("apply agent status: %w", err)
	}
	return observed, nil
}

// Statuses derives the current status for the given Agents without writing.
// Unknown Agents return their configured default (IDLE).
func (s *Service) Statuses(ctx context.Context, agentIDs []string) (map[string]agent.Observed, error) {
	if len(agentIDs) == 0 {
		return map[string]agent.Observed{}, nil
	}
	activities, err := s.repository.Activity(ctx, agentIDs)
	if err != nil {
		return nil, fmt.Errorf("read agent activity: %w", err)
	}
	now := s.now().UTC()
	observed := make(map[string]agent.Observed, len(agentIDs))
	for _, id := range agentIDs {
		observed[id] = Derive(now, s.policy, activities[id])
	}
	return observed, nil
}

// Reconcile expires silent sessions and persists observed transitions. It is
// safe to call repeatedly: unchanged statuses produce no events.
func (s *Service) Reconcile(ctx context.Context) error {
	now := s.now().UTC()
	closed, err := s.repository.CloseSessionsBefore(ctx, now.Add(-s.policy.OfflineAfter), now)
	if err != nil {
		return fmt.Errorf("expire agent sessions: %w", err)
	}
	expired := make(map[string]struct{}, len(closed))
	for _, session := range closed {
		expired[session.AgentID] = struct{}{}
	}
	agentIDs, err := s.repository.AgentsWithSessions(ctx)
	if err != nil {
		return fmt.Errorf("list agents with sessions: %w", err)
	}
	activities, err := s.repository.Activity(ctx, agentIDs)
	if err != nil {
		return fmt.Errorf("read agent activity: %w", err)
	}
	var result error
	for _, id := range agentIDs {
		activity := activities[id]
		status := Derive(now, s.policy, activity).Status
		reason := ReasonReconciled
		if _, ok := expired[id]; ok {
			reason = ReasonSessionExpired
		} else if status == agent.StatusBusy || status == agent.StatusWaiting || status == agent.StatusError {
			reason = ReasonWorkChanged
		}
		if _, _, err := s.repository.ApplyStatus(ctx, StatusChange{
			AgentID:   id,
			SessionID: activity.SessionID,
			To:        status,
			Reason:    reason,
			At:        now,
		}); err != nil {
			result = errors.Join(result, fmt.Errorf("apply agent status %s: %w", id, err))
		}
	}
	return result
}

// Events returns the recorded status transitions for one Agent, newest first.
func (s *Service) Events(ctx context.Context, agentID string, limit int) ([]StatusEvent, error) {
	if agentID == "" || len(agentID) > 200 {
		return nil, ErrInvalidInput
	}
	if limit == 0 {
		limit = defaultPage
	}
	if limit < 1 || limit > maxEvents {
		return nil, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidInput, maxEvents)
	}
	return s.repository.StatusEvents(ctx, agentID, limit)
}

func (s *Service) activityFor(ctx context.Context, agentID string) (Activity, error) {
	activities, err := s.repository.Activity(ctx, []string{agentID})
	if err != nil {
		return Activity{}, fmt.Errorf("read agent activity: %w", err)
	}
	return activities[agentID], nil
}
