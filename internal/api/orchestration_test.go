package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boAIrd/internal/brain"
	"boAIrd/internal/orchestrator"
	"boAIrd/internal/task"
)

type orchestrationAPIStub struct {
	report   orchestrator.DispatchReport
	value    task.Task
	decision orchestrator.ReviewDecision
	reason   string
	err      error
}

func (s *orchestrationAPIStub) Dispatch(context.Context, string) (orchestrator.DispatchReport, error) {
	return s.report, s.err
}

func (s *orchestrationAPIStub) Review(_ context.Context, _ string, decision orchestrator.ReviewDecision, reason string) (task.Task, error) {
	s.decision, s.reason = decision, reason
	if s.err != nil {
		return task.Task{}, s.err
	}
	return s.value, nil
}

func TestDispatchRouteReturnsReport(t *testing.T) {
	stub := &orchestrationAPIStub{report: orchestrator.DispatchReport{Outcomes: []orchestrator.ExecutionOutcome{{TaskID: "task-1", Status: orchestrator.OutcomeSucceeded}}}}
	mux := http.NewServeMux()
	registerOrchestrationRoutes(mux, stub)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/orchestration/dispatch", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"SUCCEEDED"`) {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestReviewRouteNormalizesDecision(t *testing.T) {
	value, err := task.New("task-1", "project-1", "Task", "", "admin")
	if err != nil {
		t.Fatal(err)
	}
	stub := &orchestrationAPIStub{value: value}
	mux := http.NewServeMux()
	registerOrchestrationRoutes(mux, stub)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/review", strings.NewReader(`{"decision":" rework ","reason":"add tests"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if stub.decision != orchestrator.DecisionRework || stub.reason != "add tests" {
		t.Fatalf("review request = %s/%q", stub.decision, stub.reason)
	}
	if strings.Contains(response.Body.String(), "internal_error") {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestOrchestrationErrorsMapToSafeResponses(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"human approval", brain.ErrHumanApprovalRequired, http.StatusConflict},
		{"not in review", orchestrator.ErrTaskNotInReview, http.StatusConflict},
		{"forbidden", task.ErrForbidden, http.StatusForbidden},
		{"invalid", orchestrator.ErrInvalidRequest, http.StatusBadRequest},
		{"unavailable", orchestrator.ErrProjectUnavailable, http.StatusConflict},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeOrchestrationError(response, test.err)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if strings.Contains(response.Body.String(), "test") {
				t.Fatalf("body leaked internal detail: %s", response.Body.String())
			}
		})
	}
}

func TestOrchestrationRoutesUseProjectCapabilities(t *testing.T) {
	required, projectID, recognized := requiredAPICapability(httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/orchestration/dispatch", nil), nil, nil, nil)
	if !recognized || required != "task.assign" || projectID != "project-1" {
		t.Fatalf("dispatch capability = %q project = %q recognized = %t", required, projectID, recognized)
	}
	value, err := task.New("task-1", "project-1", "Task", "", "admin")
	if err != nil {
		t.Fatal(err)
	}
	required, projectID, recognized = requiredAPICapability(httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/review", nil), nil, &taskAPIStub{value: value}, nil)
	if !recognized || required != "task.review" || projectID != "project-1" {
		t.Fatalf("review capability = %q project = %q recognized = %t", required, projectID, recognized)
	}
}
