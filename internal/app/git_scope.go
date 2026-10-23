package app

import (
	"context"
	"strings"

	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"
	"boAIrd/internal/tool"
)

type gitScopeAuthorizer struct {
	projects interface {
		Get(context.Context, string) (project.Project, error)
	}
	tasks interface {
		Get(context.Context, string) (task.Task, error)
	}
	runs interface {
		Get(context.Context, string) (run.Run, error)
	}
}

type gitTaskReader interface {
	Get(context.Context, string) (task.Task, error)
}

func gitTaskReviewApproved(ctx context.Context, tasks gitTaskReader, projectID, taskID string) (bool, error) {
	if tasks == nil {
		return false, tool.ErrDenied
	}
	value, err := tasks.Get(ctx, taskID)
	if err != nil {
		return false, err
	}
	return value.ProjectID() == projectID && value.HumanReview() && value.Status() == task.StatusDone, nil
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
	if request.TaskID != "" {
		if a.tasks == nil {
			return tool.ErrDenied
		}
		taskValue, err := a.tasks.Get(ctx, request.TaskID)
		if err != nil || taskValue.ProjectID() != request.ProjectID {
			return tool.ErrDenied
		}
	}
	if request.RunID != "" {
		if a.runs == nil {
			return tool.ErrDenied
		}
		runValue, err := a.runs.Get(ctx, request.RunID)
		if err != nil || runValue.ProjectID() != request.ProjectID || runValue.TaskID() != request.TaskID {
			return tool.ErrDenied
		}
	}
	return nil
}
