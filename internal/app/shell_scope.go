package app

import (
	"context"
	"strings"

	"boAIrd/internal/project"
	"boAIrd/internal/tool"
)

type shellScopeAuthorizer struct {
	projects interface {
		Get(context.Context, string) (project.Project, error)
	}
}

func (a shellScopeAuthorizer) Authorize(ctx context.Context, principal tool.Principal, request tool.Request) error {
	if a.projects == nil || strings.TrimSpace(request.ProjectID) == "" ||
		request.ResourceType != "workspace" || request.ResourceID != request.ProjectID ||
		strings.TrimSpace(principal.ID) == "" || !containsProject(principal.ProjectIDs, request.ProjectID) {
		return tool.ErrDenied
	}
	value, err := a.projects.Get(ctx, request.ProjectID)
	if err != nil || value.Status() == project.StatusArchived || strings.TrimSpace(value.WorkspacePath()) == "" {
		return tool.ErrDenied
	}
	return nil
}

func containsProject(projectIDs []string, projectID string) bool {
	for _, candidate := range projectIDs {
		if candidate == projectID {
			return true
		}
	}
	return false
}
