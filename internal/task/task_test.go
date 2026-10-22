package task

import (
	"errors"
	"testing"
)

func TestNewRequiresReferences(t *testing.T) {
	if _, err := New("t1", "p1", "Title", "", " "); err == nil {
		t.Fatal("expected empty creator reference to be rejected")
	}
	task, err := New("t1", "p1", "Title", "Description", "user1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusBacklog || task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() {
		t.Fatalf("unexpected task defaults: %+v", task)
	}
}

func TestTaskTransitions(t *testing.T) {
	task, _ := New("t1", "p1", "Title", "", "user1")
	for _, status := range []Status{StatusReady, StatusWorking, StatusReview, StatusHumanReview, StatusDone} {
		var err error
		task, err = task.Transition(status)
		if err != nil {
			t.Fatalf("transition to %s: %v", status, err)
		}
	}
	if _, err := task.Transition(StatusWorking); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected completed task to be terminal, got %v", err)
	}
}

func TestHumanReviewCannotBeBypassed(t *testing.T) {
	task, _ := New("t1", "p1", "Title", "", "user1")
	task.HumanReview = true
	task.Status = StatusReview
	if _, err := task.Transition(StatusDone); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected direct completion to be rejected when human review is required, got %v", err)
	}
	if _, err := task.Transition(StatusHumanReview); err != nil {
		t.Fatalf("expected required human review transition to be allowed: %v", err)
	}
}

func TestTaskRejectsSelfReferences(t *testing.T) {
	task, _ := New("t1", "p1", "Title", "", "user1")
	if _, err := task.SetReferences("t1", ""); err == nil {
		t.Fatal("expected self-assignment to be rejected")
	}
	if _, err := task.SetReferences("", "t1"); err == nil {
		t.Fatal("expected self-parent reference to be rejected")
	}
}
