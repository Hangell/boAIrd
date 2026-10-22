package app

import (
	"context"
	"errors"
	"strings"

	"boAIrd/internal/agent"
	"boAIrd/internal/runtime"
	"boAIrd/internal/task"
)

type runtimeTaskReader interface {
	Get(context.Context, string) (task.Task, error)
}

type runtimeExecutionScope struct {
	tasks runtimeTaskReader
}

func (s runtimeExecutionScope) Authorize(ctx context.Context, worker agent.Agent, request runtime.Request) error {
	projectID := strings.TrimSpace(request.ProjectID)
	if projectID == "" {
		return errors.New("project is required")
	}
	member := false
	for _, id := range worker.ProjectIDs() {
		if id == projectID {
			member = true
			break
		}
	}
	taskID := strings.TrimSpace(request.TaskID)
	if taskID == "" {
		if member {
			return nil
		}
		return errors.New("agent has no project scope")
	}
	if s.tasks == nil {
		return errors.New("task scope cannot be verified")
	}
	work, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return errors.New("assigned task scope could not be verified")
	}
	if work.ProjectID() != projectID || work.AssignedTo() != worker.ID() || work.Status() != task.StatusWorking {
		return errors.New("agent is not authorized for the requested task")
	}
	return nil
}
