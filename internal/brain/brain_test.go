package brain

import (
	"errors"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/project"
)

func TestNewValidatesReferencesAndAutonomy(t *testing.T) {
	for _, args := range [][4]string{
		{"", "p1", "a1", string(AutonomyManual)},
		{"b1", "", "a1", string(AutonomyManual)},
		{"b1", "p1", "", string(AutonomyManual)},
		{"b1", "p1", "a1", "INVALID"},
	} {
		if _, err := New(args[0], args[1], args[2], AutonomyMode(args[3])); err == nil {
			t.Fatalf("expected invalid Brain input %q to be rejected", args)
		}
	}
	brain, err := New("b1", "p1", "a1", AutonomySemiAuto)
	if err != nil {
		t.Fatal(err)
	}
	if !brain.Active() || brain.Autonomy() != AutonomySemiAuto {
		t.Fatalf("unexpected Brain defaults: active=%t autonomy=%s", brain.Active(), brain.Autonomy())
	}
	if _, err := brain.SetAutonomy("INVALID"); err == nil {
		t.Fatal("expected invalid autonomy to be rejected")
	}
	brain = brain.Deactivate()
	if brain.Active() {
		t.Fatal("expected Brain to deactivate through its method")
	}
}

func TestAuthorizeActionRequiresExplicitCapabilityRegardlessOfRoleOrAutonomy(t *testing.T) {
	p, err := project.Restore(project.Snapshot{ID: "p1", Name: "Project", Status: project.StatusConfiguring, BrainID: "b1", HumanReviewRequired: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := New("b1", "p1", "a1", AutonomyAutonomous)
	if err != nil {
		t.Fatal(err)
	}
	a, err := agent.NewConfigured("a1", agent.Configuration{Name: "Admin", Role: "administrator", Runtime: agent.RuntimeAPI, Provider: "openai", Model: "fake", CredentialID: "credential", Capabilities: []string{"task.complete"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.AuthorizeAction(ActionPlan, p, a); !errors.Is(err, ErrUnauthorizedAction) {
		t.Fatalf("plan without brain.execute: %v", err)
	}
	if err := b.AuthorizeAction(ActionCompleteTask, p, a); !errors.Is(err, ErrUnauthorizedAction) {
		t.Fatalf("completion bypassed human review: %v", err)
	}
	for _, action := range []Action{ActionReplaceBrain, ActionGrantCapability, ActionAccessCredential, ActionBypassHumanReview} {
		if err := b.AuthorizeAction(action, p, a); !errors.Is(err, ErrUnauthorizedAction) {
			t.Errorf("action %s: %v", action, err)
		}
	}
}
