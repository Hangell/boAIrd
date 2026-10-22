package runtime

import (
	"context"
	"errors"
	"strings"

	"boAIrd/internal/agent"
)

// ScopeAuthorizer checks the project and resource scope of an execution before
// the Enforcer resolves or invokes its runtime adapter.
type ScopeAuthorizer interface {
	Authorize(context.Context, agent.Agent, Request) error
}

// ProjectMembershipScope permits an execution only when the Agent has
// persisted membership in the requested project.
type ProjectMembershipScope struct{}

func (ProjectMembershipScope) Authorize(_ context.Context, a agent.Agent, request Request) error {
	projectID := strings.TrimSpace(request.ProjectID)
	if projectID == "" || !hasProjectScope(a, projectID) {
		return errors.New("agent is not authorized for project")
	}
	return nil
}
