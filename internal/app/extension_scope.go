package app

import (
	"context"
	"errors"
	"strings"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"
	governedtool "boAIrd/internal/tool"
)

type extensionScopeAuthorizer struct {
	agents interface {
		Get(context.Context, string) (agent.Agent, error)
	}
	projects interface {
		Get(context.Context, string) (project.Project, error)
	}
	brains interface {
		Get(context.Context, string) (brain.Brain, error)
	}
	tasks interface {
		Get(context.Context, string) (task.Task, error)
	}
	runs interface {
		Get(context.Context, string) (run.Run, error)
	}
	extensions interface {
		IsActiveTool(context.Context, string) (bool, error)
	}
}

func (a extensionScopeAuthorizer) Authorize(ctx context.Context, principal governedtool.Principal, request governedtool.Request) error {
	if a.agents == nil || a.projects == nil || a.brains == nil || principal.Type != "agent" || strings.TrimSpace(principal.ID) == "" || request.ResourceType != "extension" ||
		request.ResourceID == "" || request.ResourceID != request.ToolName {
		return errors.New("extension resource scope is invalid")
	}
	if a.extensions == nil {
		return errors.New("extension lifecycle resolver is unavailable")
	}
	active, err := a.extensions.IsActiveTool(ctx, request.ToolName)
	if err != nil || !active {
		return errors.New("extension tool is not active")
	}
	value, err := a.agents.Get(ctx, principal.ID)
	if err != nil || value.ID() != principal.ID {
		return errors.New("extension actor is unavailable")
	}
	projectValue, err := a.projects.Get(ctx, request.ProjectID)
	if err != nil || projectValue.Status() == project.StatusArchived {
		return errors.New("extension project is unavailable")
	}
	brainValue, brainErr := a.brains.Get(ctx, request.ProjectID)
	isProjectBrain := brainErr == nil && brainValue.Active() && brainValue.ProjectID() == request.ProjectID && brainValue.AgentID() == principal.ID
	if !hasID(value.ProjectIDs(), request.ProjectID) && !isProjectBrain {
		return errors.New("extension project membership is required")
	}
	if request.TaskID != "" {
		if a.tasks == nil {
			return errors.New("extension task resolver is unavailable")
		}
		taskValue, err := a.tasks.Get(ctx, request.TaskID)
		if err != nil || taskValue.ProjectID() != request.ProjectID {
			return errors.New("extension task belongs to another project or is unavailable")
		}
		if taskValue.AssignedTo() != principal.ID && !isProjectBrain {
			return errors.New("extension task assignment is required")
		}
	}
	if request.RunID != "" {
		if request.TaskID == "" || a.runs == nil {
			return errors.New("extension run scope requires a task")
		}
		runValue, err := a.runs.Get(ctx, request.RunID)
		if err != nil || runValue.ProjectID() != request.ProjectID || runValue.TaskID() != request.TaskID ||
			(runValue.AgentID() != principal.ID && !isProjectBrain) {
			return errors.New("extension run belongs to another project, task, or actor")
		}
	}
	return nil
}

func hasID(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
