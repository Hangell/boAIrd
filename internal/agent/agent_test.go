package agent

import "testing"

func TestAgentConstructorAndStatus(t *testing.T) {
	if _, err := New("a1", "", "worker"); err == nil {
		t.Fatal("expected empty name to be rejected")
	}
	a, err := New("a1", "Worker", "developer")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != StatusIdle {
		t.Fatalf("expected initial status %q, got %q", StatusIdle, a.Status)
	}
	if _, err := a.SetStatus("UNKNOWN"); err == nil {
		t.Fatal("expected unknown status to be rejected")
	}
}

func TestAddProjectRejectsEmptyAndAvoidsDuplicates(t *testing.T) {
	a, _ := New("a1", "Worker", "developer")
	if _, err := a.AddProject(" "); err == nil {
		t.Fatal("expected empty project reference to be rejected")
	}
	a, err := a.AddProject("p1")
	if err != nil {
		t.Fatal(err)
	}
	a, err = a.AddProject("p1")
	if err != nil || len(a.ProjectIDs) != 1 {
		t.Fatalf("expected duplicate membership to remain unique, agent=%+v err=%v", a, err)
	}
}
