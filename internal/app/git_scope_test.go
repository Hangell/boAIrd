package app

import (
	"context"
	"testing"

	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"
	"boAIrd/internal/tool"
)

type gitProjectsStub map[string]project.Project

func (s gitProjectsStub) Get(_ context.Context, id string) (project.Project, error) {
	value, ok := s[id]
	if !ok {
		return project.Project{}, project.ErrInvalidTransition
	}
	return value, nil
}

type gitTasksStub map[string]task.Task

func (s gitTasksStub) Get(_ context.Context, id string) (task.Task, error) {
	value, ok := s[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	return value, nil
}

type gitRunsStub map[string]run.Run

func (s gitRunsStub) Get(_ context.Context, id string) (run.Run, error) {
	value, ok := s[id]
	if !ok {
		return run.Run{}, run.ErrNotFound
	}
	return value, nil
}

func newGitScopeTask(t *testing.T, id, projectID string, status task.Status, humanReview bool) task.Task {
	t.Helper()
	value, err := task.NewWithActor(id, projectID, "Git operation", "", task.Actor{Type: task.ActorAdministrator, ID: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if humanReview {
		value, err = value.RequireHumanReview()
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot := value.Snapshot()
	snapshot.Status = status
	value, err = task.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestGitScopeAuthorizerValidatesTaskAndRunProjectScope(t *testing.T) {
	projectOne, err := project.Restore(project.Snapshot{
		ID: "project-1", Name: "One", Status: project.StatusCreated, WorkspacePath: "/workspace/one",
	})
	if err != nil {
		t.Fatal(err)
	}
	projectTwo, err := project.Restore(project.Snapshot{
		ID: "project-2", Name: "Two", Status: project.StatusCreated, WorkspacePath: "/workspace/two",
	})
	if err != nil {
		t.Fatal(err)
	}
	projectReader := gitProjectsStub{"project-1": projectOne, "project-2": projectTwo}
	taskOne := newGitScopeTask(t, "task-1", "project-1", task.StatusDone, true)
	taskTwo := newGitScopeTask(t, "task-2", "project-2", task.StatusDone, true)
	runOne, err := run.New("run-1", "project-1", "task-1", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	runTwo, err := run.New("run-2", "project-2", "task-2", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	authorizer := gitScopeAuthorizer{
		projects: projectReader,
		tasks:    gitTasksStub{"task-1": taskOne, "task-2": taskTwo},
		runs:     gitRunsStub{"run-1": runOne, "run-2": runTwo},
	}
	principal := tool.Principal{ID: "agent-1", Type: "agent", ProjectIDs: []string{"project-1", "project-2"}}
	request := tool.Request{ProjectID: "project-1", TaskID: "task-1", RunID: "run-1", ResourceType: "repository", ResourceID: "/workspace/one"}
	if err := authorizer.Authorize(context.Background(), principal, request); err != nil {
		t.Fatalf("valid task/run scope error = %v", err)
	}

	request.TaskID = "task-2"
	request.RunID = ""
	if err := authorizer.Authorize(context.Background(), principal, request); err == nil {
		t.Fatal("cross-project task was authorized")
	}
	request.TaskID = "task-1"
	request.RunID = "run-2"
	if err := authorizer.Authorize(context.Background(), principal, request); err == nil {
		t.Fatal("cross-project run was authorized")
	}
	request.RunID = "run-1"
	request.TaskID = "task-2"
	if err := authorizer.Authorize(context.Background(), principal, request); err == nil {
		t.Fatal("run linked to a different task was authorized")
	}
}

func TestGitTaskReviewApprovalRequiresSameProjectHumanReviewedDoneTask(t *testing.T) {
	approved := newGitScopeTask(t, "approved", "project-1", task.StatusDone, true)
	ordinary := newGitScopeTask(t, "ordinary", "project-1", task.StatusDone, false)
	reader := gitTasksStub{"approved": approved, "ordinary": ordinary}
	if ok, err := gitTaskReviewApproved(context.Background(), reader, "project-2", "approved"); err != nil || ok {
		t.Fatalf("cross-project approval = %v, error = %v; want false", ok, err)
	}
	if ok, err := gitTaskReviewApproved(context.Background(), reader, "project-1", "ordinary"); err != nil || ok {
		t.Fatalf("task without Human Review approval = %v, error = %v; want false", ok, err)
	}
	if ok, err := gitTaskReviewApproved(context.Background(), reader, "project-1", "approved"); err != nil || !ok {
		t.Fatalf("approved task = %v, error = %v; want true", ok, err)
	}
}
