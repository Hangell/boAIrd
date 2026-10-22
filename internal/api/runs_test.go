package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"boAIrd/internal/run"
)

type runAPIStub struct {
	value                      run.Run
	events                     []run.AuditEvent
	eventsByRun                map[string][]run.AuditEvent
	listValues                 []run.Run
	batchEventCalls            int
	singleEventCalls           int
	projectID, taskID, agentID string
	limit, offset              int
}

func (s *runAPIStub) Get(_ context.Context, id string) (run.Run, error) {
	if id != s.value.ID() {
		return run.Run{}, run.ErrNotFound
	}
	return s.value, nil
}
func (s *runAPIStub) List(_ context.Context, projectID, taskID, agentID string, limit, offset int) ([]run.Run, error) {
	s.projectID, s.taskID, s.agentID, s.limit, s.offset = projectID, taskID, agentID, limit, offset
	if s.listValues != nil {
		return s.listValues, nil
	}
	return []run.Run{s.value}, nil
}
func (s *runAPIStub) ListAuditEvents(context.Context, string) ([]run.AuditEvent, error) {
	s.singleEventCalls++
	return s.events, nil
}
func (s *runAPIStub) ListAuditEventsForRuns(_ context.Context, ids []string) (map[string][]run.AuditEvent, error) {
	s.batchEventCalls++
	if len(ids) != len(s.listValues) && s.listValues != nil {
		return nil, run.ErrInvalidQuery
	}
	return s.eventsByRun, nil
}

func TestRunRoutesFilterAndReturnSanitizedRunWithAudit(t *testing.T) {
	value, err := run.NewBrainRun("r1", "p1", "a1")
	if err != nil {
		t.Fatal(err)
	}
	value, _ = value.ConfigureExecution("API", "test", "model", `Objective api_key="run-secret"`)
	value, _ = value.Transition(run.StatusRunning)
	value, _ = value.SetOutput(`Authorization: Bearer output-secret`)
	value, _ = value.Transition(run.StatusSuccess)
	secondValue, err := run.NewBrainRun("r2", "p1", "a1")
	if err != nil {
		t.Fatal(err)
	}
	firstEvent := run.AuditEvent{ID: "e1", ProjectID: "p1", ActorType: "agent", ActorID: "a1", EventType: "run.status.changed", ResourceType: "run", ResourceID: "r1", ToStatus: "SUCCESS", CreatedAt: time.Now().UTC()}
	secondEvent := run.AuditEvent{ID: "e2", ProjectID: "p1", ActorType: "agent", ActorID: "a1", EventType: "run.created", ResourceType: "run", ResourceID: "r2", ToStatus: "PENDING", CreatedAt: time.Now().UTC()}
	stub := &runAPIStub{
		value: value, listValues: []run.Run{value, secondValue},
		eventsByRun: map[string][]run.AuditEvent{"r1": {firstEvent}, "r2": {secondEvent}},
	}
	mux := http.NewServeMux()
	registerRunRoutes(mux, stub)
	recorder := request(t, mux, http.MethodGet, "/api/v1/projects/p1/runs?task_id=t1&agent_id=a1&limit=1&offset=0")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if stub.projectID != "p1" || stub.taskID != "t1" || stub.agentID != "a1" || stub.limit != 1 {
		t.Fatalf("filters not forwarded: %+v", stub)
	}
	var page struct {
		Items []runResponse `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].Status != run.StatusSuccess || len(page.Items[0].AuditEvents) != 1 || len(page.Items[1].AuditEvents) != 1 {
		t.Fatalf("unexpected run page: %+v", page)
	}
	if stub.batchEventCalls != 1 || stub.singleEventCalls != 0 {
		t.Fatalf("audit event queries: batch=%d per-run=%d, want one batch query", stub.batchEventCalls, stub.singleEventCalls)
	}
	body := recorder.Body.String()
	for _, secret := range []string{"run-secret", "output-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("run response leaked %q: %s", secret, body)
		}
	}
}

func TestGetRunRouteIncludesAuditEvents(t *testing.T) {
	value, err := run.NewBrainRun("r1", "p1", "a1")
	if err != nil {
		t.Fatal(err)
	}
	event := run.AuditEvent{ID: "e1", ProjectID: "p1", ActorType: "agent", ActorID: "a1", EventType: "run.created", ResourceType: "run", ResourceID: "r1", ToStatus: "PENDING", CreatedAt: time.Now().UTC()}
	stub := &runAPIStub{value: value, events: []run.AuditEvent{event}}
	mux := http.NewServeMux()
	registerRunRoutes(mux, stub)
	recorder := request(t, mux, http.MethodGet, "/api/v1/runs/r1")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response runResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.AuditEvents) != 1 || response.AuditEvents[0].ID != "e1" {
		t.Fatalf("detail audit events = %#v", response.AuditEvents)
	}
}

func TestRunRouteRejectsInvalidPagination(t *testing.T) {
	value, _ := run.NewBrainRun("r1", "p1", "a1")
	stub := &runAPIStub{value: value}
	mux := http.NewServeMux()
	registerRunRoutes(mux, stub)
	recorder := request(t, mux, http.MethodGet, "/api/v1/projects/p1/runs?offset=-1")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}
