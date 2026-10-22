package capability

import (
	"errors"
	"testing"
)

func TestCapabilityIdentifier(t *testing.T) {
	for _, identifier := range []string{"filesystem.read", "shell.execute", "github.pr.merge"} {
		capability, err := New(identifier)
		if err != nil || !capability.Valid() {
			t.Errorf("expected %q to be valid, capability=%q err=%v", identifier, capability, err)
		}
	}
	for _, identifier := range []string{
		"", "Filesystem.Read", "filesystem-read", "filesystem_read",
		"filesystem..read", " shell.execute", "filesystem.read ",
	} {
		if _, err := New(identifier); err == nil {
			t.Errorf("expected %q to be rejected", identifier)
		}
	}
}

func TestAuthorizerRequiresExplicitCapabilityAndProjectScope(t *testing.T) {
	authorizer := Authorizer{}
	read, _ := New("task.read")
	write, _ := New("task.create")
	principal := Principal{ID: "admin", Capabilities: []Capability{read}, ProjectIDs: []string{"project-a"}}

	if err := authorizer.Authorize(principal, read, "project-a"); err != nil {
		t.Fatalf("explicit scoped capability denied: %v", err)
	}
	for _, test := range []struct {
		name      string
		principal Principal
		required  Capability
		projectID string
	}{
		{name: "capability missing", principal: principal, required: write, projectID: "project-a"},
		{name: "cross project", principal: principal, required: read, projectID: "project-b"},
		{name: "empty identity", principal: Principal{Capabilities: []Capability{read}, ProjectIDs: []string{"project-a"}}, required: read, projectID: "project-a"},
		{name: "invalid capability", principal: principal, required: Capability("Task.Read"), projectID: "project-a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := authorizer.Authorize(test.principal, test.required, test.projectID); !errors.Is(err, ErrDenied) {
				t.Fatalf("authorization error = %v, want ErrDenied", err)
			}
		})
	}
}
