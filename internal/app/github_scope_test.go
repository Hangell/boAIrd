package app

import (
	"context"
	"errors"
	"testing"

	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"
	"boAIrd/internal/tool"
)

type githubProjectsStub map[string]project.Project

func (s githubProjectsStub) Get(_ context.Context, id string) (project.Project, error) {
	value, ok := s[id]
	if !ok {
		return project.Project{}, project.ErrInvalidTransition
	}
	return value, nil
}

type githubTasksStub map[string]task.Task

func (s githubTasksStub) Get(_ context.Context, id string) (task.Task, error) {
	value, ok := s[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	return value, nil
}

type githubRunsStub map[string]run.Run

func (s githubRunsStub) Get(_ context.Context, id string) (run.Run, error) {
	value, ok := s[id]
	if !ok {
		return run.Run{}, run.ErrNotFound
	}
	return value, nil
}

func newGitHubScopeTask(t *testing.T, id, projectID string, status task.Status, humanReview bool) task.Task {
	t.Helper()
	value, err := task.NewWithActor(id, projectID, "GitHub operation", "", task.Actor{Type: task.ActorAdministrator, ID: "admin"})
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

func TestGitHubScopeAuthorizerValidatesRepositoryTaskAndRunScope(t *testing.T) {
	projectOne, err := project.Restore(project.Snapshot{
		ID: "project-1", Name: "One", Status: project.StatusCreated, RepositoryURL: "https://github.com/acme/widgets",
	})
	if err != nil {
		t.Fatal(err)
	}
	projectTwo, err := project.Restore(project.Snapshot{
		ID: "project-2", Name: "Two", Status: project.StatusCreated, RepositoryURL: "https://github.com/acme/other",
	})
	if err != nil {
		t.Fatal(err)
	}
	taskOne := newGitHubScopeTask(t, "task-1", "project-1", task.StatusDone, true)
	taskTwo := newGitHubScopeTask(t, "task-2", "project-2", task.StatusDone, true)
	runOne, err := run.New("run-1", "project-1", "task-1", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	authorizer := githubScopeAuthorizer{
		projects: githubProjectsStub{"project-1": projectOne, "project-2": projectTwo},
		tasks:    githubTasksStub{"task-1": taskOne, "task-2": taskTwo},
		runs:     githubRunsStub{"run-1": runOne},
	}
	principal := tool.Principal{ID: "agent-1", ProjectIDs: []string{"project-1"}}
	base := tool.Request{ProjectID: "project-1", ResourceType: "github.repository", ResourceID: "https://github.com/acme/widgets"}

	if err := authorizer.Authorize(context.Background(), principal, base); err != nil {
		t.Fatalf("authorize() error = %v, want nil", err)
	}
	// Cross-project task reference is denied.
	withTask := base
	withTask.TaskID = "task-2"
	if err := authorizer.Authorize(context.Background(), principal, withTask); !errors.Is(err, tool.ErrDenied) {
		t.Fatalf("authorize(cross-project task) error = %v, want denied", err)
	}
	// Run referencing a different task is denied.
	withRun := base
	withRun.TaskID = "task-1"
	withRun.RunID = "run-1"
	if err := authorizer.Authorize(context.Background(), principal, withRun); err != nil {
		t.Fatalf("authorize(matching run) error = %v, want nil", err)
	}
	withRun.RunID = "missing"
	if err := authorizer.Authorize(context.Background(), principal, withRun); !errors.Is(err, tool.ErrDenied) {
		t.Fatalf("authorize(missing run) error = %v, want denied", err)
	}
	// Different repository binding is denied.
	wrongRepo := base
	wrongRepo.ResourceID = "https://github.com/acme/other"
	if err := authorizer.Authorize(context.Background(), principal, wrongRepo); !errors.Is(err, tool.ErrDenied) {
		t.Fatalf("authorize(wrong repository) error = %v, want denied", err)
	}
}

func TestGitHubTaskReviewApprovedRequiresProjectAndDone(t *testing.T) {
	tasks := githubTasksStub{
		"approved": newGitHubScopeTask(t, "approved", "project-1", task.StatusDone, true),
		"pending":  newGitHubScopeTask(t, "pending", "project-1", task.StatusHumanReview, true),
		"other":    newGitHubScopeTask(t, "other", "project-2", task.StatusDone, true),
	}
	approved, err := githubTaskReviewApproved(context.Background(), tasks, "project-1", "approved")
	if err != nil || !approved {
		t.Fatalf("approved task = %t, error = %v; want true", approved, err)
	}
	approved, err = githubTaskReviewApproved(context.Background(), tasks, "project-1", "pending")
	if err != nil || approved {
		t.Fatalf("pending review task = %t, error = %v; want false", approved, err)
	}
	approved, err = githubTaskReviewApproved(context.Background(), tasks, "project-1", "other")
	if err != nil || approved {
		t.Fatalf("cross-project task = %t, error = %v; want false", approved, err)
	}
}
