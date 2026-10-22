package task

import (
	"errors"
	"testing"
)

func TestNewRequiresReferencesAndInitializesBacklog(t *testing.T) {
	if _, err := New("t1", "p1", "Title", "", " "); err == nil {
		t.Fatal("expected empty creator reference to be rejected")
	}
	task, err := New("t1", "p1", "Title", "Description", "user1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status() != StatusBacklog || task.CreatedAt().IsZero() || task.UpdatedAt().IsZero() {
		t.Fatalf("unexpected task defaults: status=%s created=%s updated=%s", task.Status(), task.CreatedAt(), task.UpdatedAt())
	}
}

func TestTaskTransitionsAndTerminalState(t *testing.T) {
	task, _ := New("t1", "p1", "Title", "", "user1")
	for _, status := range []Status{StatusReady, StatusWorking, StatusReview, StatusDone} {
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

func TestTerminalTasksRejectAssignmentChanges(t *testing.T) {
	for _, terminal := range []Status{StatusDone, StatusCancelled} {
		t.Run(string(terminal), func(t *testing.T) {
			value, err := New("t1", "p1", "Title", "", "user1")
			if err != nil {
				t.Fatal(err)
			}
			value, err = value.Assign("agent-1")
			if err != nil {
				t.Fatal(err)
			}
			if terminal == StatusDone {
				for _, next := range []Status{StatusReady, StatusWorking, StatusReview, StatusDone} {
					value, err = value.Transition(next)
					if err != nil {
						t.Fatalf("transition to %s: %v", next, err)
					}
				}
			} else {
				value, err = value.Transition(StatusCancelled)
				if err != nil {
					t.Fatal(err)
				}
			}

			if _, err := value.Assign("agent-2"); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("Assign() error = %v, want invalid transition", err)
			}
			if _, err := value.Unassign(); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("Unassign() error = %v, want invalid transition", err)
			}
			if _, err := value.SetReferences("agent-2", ""); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("SetReferences() error = %v, want invalid transition", err)
			}
			if value.AssignedTo() != "agent-1" {
				t.Fatalf("terminal task assignment changed to %q", value.AssignedTo())
			}
		})
	}
}

func TestHumanReviewCannotBeBypassed(t *testing.T) {
	task, _ := New("t1", "p1", "Title", "", "user1")
	var err error
	task, err = task.RequireHumanReview()
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []Status{StatusReady, StatusWorking, StatusReview} {
		task, err = task.Transition(status)
		if err != nil {
			t.Fatalf("transition to %s: %v", status, err)
		}
	}
	if _, err := task.Transition(StatusDone); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected direct completion to be rejected, got %v", err)
	}
	if _, err := task.DisableHumanReview(); err == nil {
		t.Fatal("expected human review to remain required after reaching review")
	}
	task, err = task.Transition(StatusHumanReview)
	if err != nil {
		t.Fatal(err)
	}
	task, err = task.Transition(StatusDone)
	if err != nil || task.Status() != StatusDone {
		t.Fatalf("expected completion after human review, task status=%s err=%v", task.Status(), err)
	}
}

func TestAssignmentAllowsAgentIDEqualToTaskID(t *testing.T) {
	task, _ := New("t1", "p1", "Title", "", "user1")
	task, err := task.Assign("t1")
	if err != nil {
		t.Fatal(err)
	}
	if task.AssignedTo() != "t1" {
		t.Fatalf("expected Agent ID to be assigned, got %q", task.AssignedTo())
	}
}

func TestTaskRejectsOnlySelfParentReference(t *testing.T) {
	task, _ := New("t1", "p1", "Title", "", "user1")
	task, err := task.SetReferences("t1", "")
	if err != nil || task.AssignedTo() != "t1" {
		t.Fatalf("expected assignment matching Task ID to be valid, task assignment=%q err=%v", task.AssignedTo(), err)
	}
	if _, err := task.SetParent("t1"); err == nil {
		t.Fatal("expected a Task to reject itself as parent")
	}
	if _, err := task.SetReferences("agent-1", "t1"); err == nil {
		t.Fatal("expected self-parent reference to be rejected")
	}
}
