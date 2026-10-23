package app

import (
	"context"
	"strings"

	"boAIrd/internal/project"
	"boAIrd/internal/tool"
)

type gitScopeAuthorizer struct {
	projects interface {
		Get(context.Context, string) (project.Project, error)
	}
}

func (a gitScopeAuthorizer) Authorize(ctx context.Context, principal tool.Principal, request tool.Request) error {
	if a.projects == nil || strings.TrimSpace(request.ProjectID) == "" ||
		request.ResourceType != "repository" || strings.TrimSpace(request.ResourceID) == "" ||
		strings.TrimSpace(principal.ID) == "" || !containsProject(principal.ProjectIDs, request.ProjectID) {
		return tool.ErrDenied
	}
	value, err := a.projects.Get(ctx, request.ProjectID)
	if err != nil || value.Status() == project.StatusArchived || strings.TrimSpace(value.WorkspacePath()) == "" {
		return tool.ErrDenied
	}
	if request.ResourceID != value.WorkspacePath() {
		return tool.ErrDenied
	}
	return nil
}
