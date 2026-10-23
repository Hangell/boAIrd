package notification

import (
	"context"
	"testing"

	"boAIrd/internal/storage"
)

type repositoryStub struct {
	pending  []storage.Notification
	outcomes []error
}

func (r *repositoryStub) List(context.Context, string, int, int) ([]storage.Notification, error) {
	return nil, nil
}
func (r *repositoryStub) MarkRead(context.Context, string, string) error    { return nil }
func (r *repositoryStub) Acknowledge(context.Context, string, string) error { return nil }
func (r *repositoryStub) Retry(context.Context, string, string) error       { return nil }
func (r *repositoryStub) Pending(context.Context, int) ([]storage.Notification, error) {
	return r.pending, nil
}
func (r *repositoryStub) RecordDelivery(_ context.Context, _ string, err error) error {
	r.outcomes = append(r.outcomes, err)
	return nil
}

func TestDispatchDoesNotSpendRetryWithoutSubscriber(t *testing.T) {
	repository := &repositoryStub{pending: []storage.Notification{{ID: "n1", ProjectID: "p1"}}}
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repository.outcomes) != 0 {
		t.Fatalf("delivery attempt recorded without subscriber: %#v", repository.outcomes)
	}
	stream, unsubscribe := service.Subscribe("p2")
	defer unsubscribe()
	if err := service.Dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stream:
		t.Fatal("notification crossed project scope")
	default:
	}
	stream, unsubscribe = service.Subscribe("p1")
	defer unsubscribe()
	if err := service.Dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case notice := <-stream:
		if notice.ID != "n1" {
			t.Fatalf("notice ID = %q", notice.ID)
		}
	default:
		t.Fatal("project subscriber did not receive notification")
	}
	if len(repository.outcomes) != 1 || repository.outcomes[0] != nil {
		t.Fatalf("successful delivery outcomes = %#v, want one success", repository.outcomes)
	}
}
