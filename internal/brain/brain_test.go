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
	if err := b.AuthorizeAction(ActionCompleteTask, p, a); !errors.Is(err, ErrHumanApprovalRequired) {
		t.Fatalf("completion bypassed human review: %v", err)
	}
	for _, action := range []Action{ActionReplaceBrain, ActionGrantCapability, ActionAccessCredential, ActionBypassHumanReview} {
		if err := b.AuthorizeAction(action, p, a); !errors.Is(err, ErrUnauthorizedAction) {
			t.Errorf("action %s: %v", action, err)
		}
	}
}

func TestAutonomyModesGateDecisionsWithoutGrantingCapabilities(t *testing.T) {
	makeProject := func(humanReview bool) project.Project {
		value, err := project.Restore(project.Snapshot{ID: "p1", Name: "Project", Status: project.StatusConfiguring, BrainID: "b1", HumanReviewRequired: humanReview})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	a, err := agent.NewConfigured("a1", agent.Configuration{
		Name: "Brain", Role: "coordinator", Runtime: agent.RuntimeAPI, Provider: "openai", Model: "fake", CredentialID: "credential",
		Capabilities: []string{"brain.execute", "task.create", "task.assign", "task.review", "task.complete"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		mode        AutonomyMode
		action      Action
		humanReview bool
		wantErr     error
	}{
		{name: "manual planning remains available", mode: AutonomyManual, action: ActionPlan},
		{name: "manual task mutation requires a human", mode: AutonomyManual, action: ActionCreateTask, wantErr: ErrHumanApprovalRequired},
		{name: "semi-auto can create tasks", mode: AutonomySemiAuto, action: ActionCreateTask},
		{name: "semi-auto completion requires a human", mode: AutonomySemiAuto, action: ActionCompleteTask, wantErr: ErrHumanApprovalRequired},
		{name: "autonomous completion follows project policy", mode: AutonomyAutonomous, action: ActionCompleteTask},
		{name: "autonomous mode respects human review", mode: AutonomyAutonomous, action: ActionCompleteTask, humanReview: true, wantErr: ErrHumanApprovalRequired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b, err := New("b1", "p1", "a1", test.mode)
			if err != nil {
				t.Fatal(err)
			}
			err = b.AuthorizeAction(test.action, makeProject(test.humanReview), a)
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("authorization error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && err != nil {
				t.Fatalf("authorization unexpectedly failed: %v", err)
			}
		})
	}

	withoutGrant, err := agent.NewConfigured("a1", agent.Configuration{
		Name: "Brain", Role: "administrator", Runtime: agent.RuntimeAPI, Provider: "openai", Model: "fake", CredentialID: "credential",
		Capabilities: []string{"brain.execute"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []AutonomyMode{AutonomyManual, AutonomySemiAuto, AutonomyAutonomous} {
		b, err := New("b1", "p1", "a1", mode)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.AuthorizeAction(ActionCreateTask, makeProject(false), withoutGrant); !errors.Is(err, ErrUnauthorizedAction) {
			t.Errorf("mode %s granted task.create without capability: %v", mode, err)
		}
	}
}

func TestMCPBrainKeepsExplicitCapabilitiesAndCannotEscalateAuthority(t *testing.T) {
	a, err := agent.NewConfigured("mcp-brain", agent.Configuration{
		Name: "External Brain", Role: "administrator", Runtime: agent.RuntimeMCP,
		Capabilities: []string{"brain.execute"},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := New("brain-1", "p1", a.ID(), AutonomyAutonomous)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.AuthorizeAction(ActionPlan, projectForBrainTest(t, "brain-1", false), a); err != nil {
		t.Fatalf("MCP Brain planning denied: %v", err)
	}
	for _, action := range []Action{ActionReplaceBrain, ActionGrantCapability, ActionAccessCredential, ActionBypassHumanReview} {
		if err := b.AuthorizeAction(action, projectForBrainTest(t, "brain-1", false), a); !errors.Is(err, ErrUnauthorizedAction) {
			t.Errorf("MCP Brain action %s error = %v, want ErrUnauthorizedAction", action, err)
		}
	}
}

func projectForBrainTest(t *testing.T, brainID string, humanReview bool) project.Project {
	t.Helper()
	value, err := project.Restore(project.Snapshot{ID: "p1", Name: "Project", Status: project.StatusConfiguring, BrainID: brainID, HumanReviewRequired: humanReview})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
