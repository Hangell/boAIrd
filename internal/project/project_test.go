package project

import "testing"

func TestNewRequiresIDAndName(t *testing.T) {
	if _, err := New("", "Project"); err == nil {
		t.Fatal("expected missing ID to be rejected")
	}
	if _, err := New("p1", " "); err == nil {
		t.Fatal("expected missing name to be rejected")
	}
}

func TestProjectRequiresBrainBeforeReady(t *testing.T) {
	p, err := New("p1", "Project")
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.Transition(StatusConfiguring)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Transition(StatusReady); err == nil {
		t.Fatal("expected project without a Brain to be rejected")
	}
	p, err = p.WithBrain("b1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Transition(StatusReady); err != nil {
		t.Fatalf("expected project with a Brain to become ready: %v", err)
	}
}

func TestArchivedProjectIsTerminal(t *testing.T) {
	p := Project{ID: "p1", Name: "Project", Status: StatusArchived}
	if _, err := p.Transition(StatusReady); err == nil {
		t.Fatal("expected archived project transition to be rejected")
	}
}
