package notification

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"boAIrd/internal/storage"
)

var ErrNotFound = storage.ErrNotificationNotFound
var ErrRetryLimit = storage.ErrNotificationRetryLimit
var ErrNoSubscribers = errors.New("no notification stream is connected")

type Repository interface {
	List(context.Context, string, int, int) ([]storage.Notification, error)
	MarkRead(context.Context, string, string) error
	Acknowledge(context.Context, string, string) error
	Retry(context.Context, string, string) error
	Pending(context.Context, int) ([]storage.Notification, error)
	RecordDelivery(context.Context, string, error) error
}

type Notice = storage.Notification

type subscriber struct{ channel chan storage.Notification }

// Service persists notification state and fans new notices out to connected dashboards.
type Service struct {
	repository  Repository
	mu          sync.Mutex
	subscribers map[string]map[*subscriber]struct{}
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("notification repository is required")
	}
	return &Service{repository: repository, subscribers: make(map[string]map[*subscriber]struct{})}, nil
}

func (s *Service) List(ctx context.Context, projectID string, limit, offset int) ([]Notice, error) {
	return s.repository.List(ctx, projectID, limit, offset)
}
func (s *Service) MarkRead(ctx context.Context, projectID, id string) error {
	return s.repository.MarkRead(ctx, projectID, id)
}
func (s *Service) Acknowledge(ctx context.Context, projectID, id string) error {
	return s.repository.Acknowledge(ctx, projectID, id)
}
func (s *Service) Retry(ctx context.Context, projectID, id string) error {
	return s.repository.Retry(ctx, projectID, id)
}

func (s *Service) Subscribe(projectID string) (<-chan storage.Notification, func()) {
	value := &subscriber{channel: make(chan storage.Notification, 16)}
	s.mu.Lock()
	if s.subscribers[projectID] == nil {
		s.subscribers[projectID] = make(map[*subscriber]struct{})
	}
	s.subscribers[projectID][value] = struct{}{}
	s.mu.Unlock()
	return value.channel, func() {
		s.mu.Lock()
		delete(s.subscribers[projectID], value)
		if len(s.subscribers[projectID]) == 0 {
			delete(s.subscribers, projectID)
		}
		s.mu.Unlock()
	}
}

func (s *Service) Publish(projectID string, value storage.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.subscribers[projectID]) == 0 {
		return ErrNoSubscribers
	}
	for target := range s.subscribers[projectID] {
		if len(target.channel) == cap(target.channel) {
			return errors.New("notification stream is full")
		}
	}
	for target := range s.subscribers[projectID] {
		target.channel <- value
	}
	return nil
}

// Run retries pending stream delivery a bounded number of times; inbox records remain durable.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.Dispatch(ctx); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "dispatch project notifications", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) Dispatch(ctx context.Context) error {
	values, err := s.repository.Pending(ctx, 100)
	if err != nil {
		return err
	}
	var result error
	for _, value := range values {
		deliveryErr := s.Publish(value.ProjectID, value)
		if errors.Is(deliveryErr, ErrNoSubscribers) {
			continue
		}
		if err := s.repository.RecordDelivery(ctx, value.ID, deliveryErr); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}
