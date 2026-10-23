package app

import (
	"context"
	"errors"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"
	governedtool "boAIrd/internal/tool"
)

type extensionAgentReader map[string]agent.Agent

func (r extensionAgentReader) Get(_ context.Context, id string) (agent.Agent, error) {
	value, ok := r[id]
	if !ok {
		return agent.Agent{}, agent.ErrNotFound
	}
	return value, nil
}

type extensionProjectReader map[string]project.Project

func (r extensionProjectReader) Get(_ context.Context, id string) (project.Project, error) {
	value, ok := r[id]
	if !ok {
		return project.Project{}, project.ErrNotFound
	}
	return value, nil
}

type extensionBrainReader map[string]brain.Brain

func (r extensionBrainReader) Get(_ context.Context, id string) (brain.Brain, error) {
	value, ok := r[id]
	if !ok {
		return brain.Brain{}, brain.ErrNotFound
	}
	return value, nil
}

type extensionTaskReader map[string]task.Task

func (r extensionTaskReader) Get(_ context.Context, id string) (task.Task, error) {
	value, ok := r[id]
	if !ok {
		return task.Task{}, task.ErrInvalidTransition
	}
	return value, nil
}

type extensionRunReader map[string]run.Run

func (r extensionRunReader) Get(_ context.Context, id string) (run.Run, error) {
	value, ok := r[id]
	if !ok {
		return run.Run{}, run.ErrNotFound
	}
	return value, nil
}

type extensionActiveToolStub bool

func (s extensionActiveToolStub) IsActiveTool(context.Context, string) (bool, error) {
	return bool(s), nil
}

func TestExtensionScopeAuthorizerChecksActiveProjectTaskAndRun(t *testing.T) {
	worker, err := agent.New("worker-1", "Worker", "backend")
	if err != nil {
		t.Fatal(err)
	}
	worker, err = worker.AddProject("project-one")
	if err != nil {
		t.Fatal(err)
	}
	brainAgent, err := agent.New("brain-agent", "Brain Agent", "brain")
	if err != nil {
		t.Fatal(err)
	}
	one, err := project.Restore(project.Snapshot{ID: "project-one", Name: "One", Status: project.StatusConfiguring})
	if err != nil {
		t.Fatal(err)
	}
	two, err := project.Restore(project.Snapshot{ID: "project-two", Name: "Two", Status: project.StatusConfiguring})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := project.Restore(project.Snapshot{ID: "project-archived", Name: "Archived", Status: project.StatusArchived})
	if err != nil {
		t.Fatal(err)
	}
	brainValue, err := brain.New("brain-one", "project-two", brainAgent.ID(), brain.AutonomySemiAuto)
	if err != nil {
		t.Fatal(err)
	}
	assignedTask, err := task.New("task-one", "project-one", "Task", "Details", worker.ID())
	if err != nil {
		t.Fatal(err)
	}
	assignedTask, err = assignedTask.Assign(worker.ID())
	if err != nil {
		t.Fatal(err)
	}
	foreignTask, err := task.New("task-two", "project-two", "Foreign task", "Details", worker.ID())
	if err != nil {
		t.Fatal(err)
	}
	runValue, err := run.New("run-one", "project-one", "task-one", worker.ID())
	if err != nil {
		t.Fatal(err)
	}
	foreignRun, err := run.New("run-two", "project-two", "task-two", worker.ID())
	if err != nil {
		t.Fatal(err)
	}
	authorizer := extensionScopeAuthorizer{
		agents:   extensionAgentReader{worker.ID(): worker, brainAgent.ID(): brainAgent},
		projects: extensionProjectReader{"project-one": one, "project-two": two, "project-archived": archived},
		brains:   extensionBrainReader{"project-two": brainValue},
		tasks:    extensionTaskReader{"task-one": assignedTask, "task-two": foreignTask},
		runs:     extensionRunReader{"run-one": runValue, "run-two": foreignRun}, extensions: extensionActiveToolStub(true),
	}
	request := governedtool.Request{
		ToolName: "example.search", ProjectID: "project-one", TaskID: "task-one", RunID: "run-one",
		ResourceType: "extension", ResourceID: "example.search",
	}
	principal := governedtool.Principal{ID: worker.ID(), Type: "agent", ProjectIDs: []string{"project-one"}}
	if err := authorizer.Authorize(context.Background(), principal, request); err != nil {
		t.Fatalf("valid extension scope denied: %v", err)
	}
	tests := []struct {
		name      string
		principal governedtool.Principal
		request   governedtool.Request
		active    extensionActiveToolStub
	}{
		{name: "inactive extension", principal: principal, request: request, active: false},
		{name: "cross-project task", principal: principal, request: func() governedtool.Request {
			value := request
			value.TaskID = "task-two"
			value.RunID = ""
			return value
		}(), active: true},
		{name: "cross-project run", principal: principal, request: func() governedtool.Request { value := request; value.RunID = "run-two"; return value }(), active: true},
		{name: "archived project", principal: governedtool.Principal{ID: worker.ID(), Type: "agent", ProjectIDs: []string{"project-archived"}}, request: func() governedtool.Request {
			value := request
			value.ProjectID = "project-archived"
			value.TaskID = ""
			value.RunID = ""
			return value
		}(), active: true},
		{name: "foreign project without membership", principal: principal, request: func() governedtool.Request {
			value := request
			value.ProjectID = "project-two"
			value.TaskID = ""
			value.RunID = ""
			return value
		}(), active: true},
		{name: "resource identity mismatch", principal: principal, request: func() governedtool.Request { value := request; value.ResourceID = "other.tool"; return value }(), active: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := authorizer
			value.extensions = test.active
			if err := value.Authorize(context.Background(), test.principal, test.request); err == nil {
				t.Fatal("invalid extension scope was accepted")
			}
		})
	}
	brainPrincipal := governedtool.Principal{ID: brainAgent.ID(), Type: "agent"}
	brainRequest := governedtool.Request{ToolName: "example.search", ProjectID: "project-two", ResourceType: "extension", ResourceID: "example.search"}
	if err := authorizer.Authorize(context.Background(), brainPrincipal, brainRequest); err != nil {
		t.Fatalf("active Brain project scope denied: %v", err)
	}
	broken := authorizer
	broken.extensions = nil
	if err := broken.Authorize(context.Background(), principal, request); err == nil || errors.Is(err, governedtool.ErrDenied) {
		t.Fatalf("missing active extension resolver error = %v, want fail-closed error", err)
	}
}
