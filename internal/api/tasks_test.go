package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boAIrd/internal/task"
)

type taskAPIStub struct {
	actor task.Actor
	value task.Task
}

func (s *taskAPIStub) Create(_ context.Context, projectID string, input task.CreateInput, actor task.Actor) (task.Task, error) {
	s.actor = actor
	value, err := task.NewWithActor("task-1", projectID, input.Title, input.Description, actor)
	s.value = value
	return value, err
}
func (s *taskAPIStub) Get(context.Context, string) (task.Task, error) { return s.value, nil }
func (s *taskAPIStub) List(context.Context, string, task.Status, string, int, int) ([]task.Task, error) {
	return []task.Task{s.value}, nil
}
func (s *taskAPIStub) Assign(context.Context, string, string, string, task.Actor) (task.Task, error) {
	return s.value, nil
}
func (s *taskAPIStub) Transition(context.Context, string, task.Status, string, task.Actor) (task.Task, error) {
	return s.value, nil
}
func (s *taskAPIStub) ListAuditEvents(context.Context, string) ([]task.AuditEvent, error) {
	return nil, nil
}

func TestCreateTaskUsesServerOwnedLocalActor(t *testing.T) {
	manager := &taskAPIStub{}
	mux := http.NewServeMux()
	registerTaskRoutes(mux, manager)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/tasks", strings.NewReader(`{"title":"Task","reason":"created locally"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if manager.actor.Type != task.ActorLocalAPI || manager.actor.ID != localTaskActorID {
		t.Fatalf("actor = %+v, want server-owned local API actor", manager.actor)
	}
}

func TestCreateTaskRejectsCallerSuppliedActor(t *testing.T) {
	manager := &taskAPIStub{}
	mux := http.NewServeMux()
	registerTaskRoutes(mux, manager)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/tasks", strings.NewReader(`{"title":"Task","reason":"created locally","actor":"administrator"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if manager.actor.ID != "" {
		t.Fatalf("manager received caller-supplied actor: %+v", manager.actor)
	}
}

func TestTaskConflictUsesRetryableConflictResponse(t *testing.T) {
	response := httptest.NewRecorder()
	writeTaskError(response, task.ErrConflict)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusConflict)
	}
	if !strings.Contains(response.Body.String(), "task changed concurrently; reload and retry") {
		t.Fatalf("body = %s, want safe conflict guidance", response.Body.String())
	}
}

func TestTaskResponseIncludesVersion(t *testing.T) {
	value, err := task.New("task-1", "project-1", "Task", "", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if response := toTaskResponse(value, nil); response.Version != 1 {
		t.Fatalf("task response version = %d, want 1", response.Version)
	}
}
