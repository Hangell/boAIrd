package agent

import (
	"testing"

	"boAIrd/internal/capability"
)

func TestAgentConstructorAndStatus(t *testing.T) {
	if _, err := New("a1", "", "worker"); err == nil {
		t.Fatal("expected empty name to be rejected")
	}
	a, err := New("a1", "Worker", "developer")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status() != StatusIdle {
		t.Fatalf("expected initial status %q, got %q", StatusIdle, a.Status())
	}
	if _, err := a.SetStatus("UNKNOWN"); err == nil {
		t.Fatal("expected unknown status to be rejected")
	}
	a, err = a.SetStatus(StatusOnline)
	if err != nil || a.Status() != StatusOnline {
		t.Fatalf("expected valid status change, status=%s err=%v", a.Status(), err)
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
	if err != nil || len(a.ProjectIDs()) != 1 {
		t.Fatalf("expected duplicate membership to remain unique, agent=%+v err=%v", a, err)
	}
}

func TestAddCapabilityValidatesAndReturnsCopy(t *testing.T) {
	a, _ := New("a1", "Worker", "developer")
	capabilityValue, err := capability.New("filesystem.read")
	if err != nil {
		t.Fatal(err)
	}
	a, err = a.AddCapability(capabilityValue)
	if err != nil {
		t.Fatal(err)
	}
	values := a.Capabilities()
	values[0] = "github.pr.merge"
	if a.Capabilities()[0] != capabilityValue {
		t.Fatal("capability getter exposed mutable internal state")
	}
}

func TestAddToolValidatesAndReturnsCopy(t *testing.T) {
	a, _ := New("a1", "Worker", "developer")
	if _, err := a.AddTool(" "); err == nil {
		t.Fatal("expected empty tool name to be rejected")
	}
	a, err := a.AddTool("filesystem.read")
	if err != nil {
		t.Fatal(err)
	}
	tools := a.Tools()
	tools[0] = "shell.execute"
	if a.Tools()[0] != "filesystem.read" {
		t.Fatal("tool getter exposed mutable internal state")
	}
}
