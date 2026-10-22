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

func TestProjectRequiresBrainBeforeReadyOrRunning(t *testing.T) {
	p, err := New("p1", "Project")
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.Transition(StatusConfiguring)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Transition(StatusReady); err == nil {
		t.Fatal("expected project without a Brain to be rejected from READY")
	}
	if _, err := p.Transition(StatusRunning); err == nil {
		t.Fatal("expected project without a Brain to be rejected from RUNNING")
	}
	p, err = p.WithBrain("b1")
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.Transition(StatusReady)
	if err != nil {
		t.Fatalf("expected project with a Brain to become ready: %v", err)
	}
	if _, err := p.WithoutBrain(); err == nil {
		t.Fatal("expected a ready project to retain its Brain")
	}
	p, err = p.Transition(StatusRunning)
	if err != nil || p.Status() != StatusRunning {
		t.Fatalf("expected project with a Brain to run, status=%s err=%v", p.Status(), err)
	}
	p, err = p.Transition(StatusPaused)
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.WithoutBrain()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Transition(StatusRunning); err == nil {
		t.Fatal("expected paused project without a Brain to be rejected from RUNNING")
	}
}

func TestArchivedProjectIsTerminal(t *testing.T) {
	p, err := New("p1", "Project")
	if err != nil {
		t.Fatal(err)
	}
	p, err = p.Transition(StatusArchived)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Transition(StatusReady); err == nil {
		t.Fatal("expected archived project transition to be rejected")
	}
}
