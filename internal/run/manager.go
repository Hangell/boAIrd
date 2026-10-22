package run

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidQuery = errors.New("invalid run query")

const (
	defaultPageSize = 50
	maximumPageSize = 100
)

type QueryRepository interface {
	Get(context.Context, string) (Run, error)
	List(context.Context, string, string, string, int, int) ([]Run, error)
	ListAuditEvents(context.Context, string) ([]AuditEvent, error)
	ListAuditEventsForRuns(context.Context, []string) (map[string][]AuditEvent, error)
}

type Manager struct{ repository QueryRepository }

func NewManager(repository QueryRepository) (*Manager, error) {
	if repository == nil {
		return nil, errors.New("run repository is required")
	}
	return &Manager{repository: repository}, nil
}

func (m *Manager) Get(ctx context.Context, id string) (Run, error) {
	if strings.TrimSpace(id) == "" {
		return Run{}, fmt.Errorf("%w: run ID is required", ErrInvalidQuery)
	}
	return m.repository.Get(ctx, id)
}

func (m *Manager) List(ctx context.Context, projectID, taskID, agentID string, limit, offset int) ([]Run, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("%w: project ID is required", ErrInvalidQuery)
	}
	if limit == 0 {
		limit = defaultPageSize
	}
	if limit < 1 || limit > maximumPageSize || offset < 0 {
		return nil, fmt.Errorf("%w: limit must be 1 to %d and offset must be non-negative", ErrInvalidQuery, maximumPageSize)
	}
	return m.repository.List(ctx, projectID, strings.TrimSpace(taskID), strings.TrimSpace(agentID), limit, offset)
}

func (m *Manager) ListAuditEvents(ctx context.Context, runID string) ([]AuditEvent, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("%w: run ID is required", ErrInvalidQuery)
	}
	if _, err := m.repository.Get(ctx, runID); err != nil {
		return nil, err
	}
	return m.repository.ListAuditEvents(ctx, runID)
}

func (m *Manager) ListAuditEventsForRuns(ctx context.Context, runIDs []string) (map[string][]AuditEvent, error) {
	if len(runIDs) == 0 {
		return map[string][]AuditEvent{}, nil
	}
	return m.repository.ListAuditEventsForRuns(ctx, runIDs)
}
