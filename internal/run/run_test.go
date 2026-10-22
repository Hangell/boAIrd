package run

import (
	"errors"
	"testing"
)

func TestRunRequiresReferences(t *testing.T) {
	if _, err := New("r1", "p1", "", "a1"); err == nil {
		t.Fatal("expected missing task reference to be rejected")
	}
	r, err := New("r1", "p1", "t1", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status() != StatusPending {
		t.Fatalf("expected initial status %q, got %q", StatusPending, r.Status())
	}
}

func TestRunTransitions(t *testing.T) {
	r, _ := New("r1", "p1", "t1", "a1")
	r, err := r.Transition(StatusRunning)
	if err != nil {
		t.Fatal(err)
	}
	if r.StartedAt().IsZero() {
		t.Fatal("expected running transition to set start time")
	}
	r, err = r.Transition(StatusSuccess)
	if err != nil {
		t.Fatal(err)
	}
	if r.FinishedAt().IsZero() {
		t.Fatal("expected terminal transition to set finish time")
	}
	if _, err := r.Transition(StatusRunning); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected successful run to be terminal, got %v", err)
	}
}

func TestRunRecordsToolsOnlyWhileRunning(t *testing.T) {
	r, _ := New("r1", "p1", "t1", "a1")
	if _, err := r.RecordToolUse("search"); err == nil {
		t.Fatal("expected tool use on a pending run to be rejected")
	}
	r, _ = r.Transition(StatusRunning)
	r, err := r.RecordToolUse("search")
	if err != nil {
		t.Fatal(err)
	}
	usedTools := r.UsedTools()
	if len(usedTools) != 1 || usedTools[0] != "search" {
		t.Fatalf("expected search tool to be recorded, got %v", usedTools)
	}
	usedTools[0] = "mutated"
	if got := r.UsedTools()[0]; got != "search" {
		t.Fatalf("mutating returned tools changed recorded tool to %q", got)
	}
	if _, err := r.RecordToolUse(" "); err == nil {
		t.Fatal("expected empty tool name to be rejected")
	}
}
