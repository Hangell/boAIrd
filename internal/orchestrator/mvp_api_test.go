package orchestrator_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boAIrd/internal/api"
	"boAIrd/internal/task"
)

const mvpAdminToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestMVPWorkflowIsVisibleThroughREST(t *testing.T) {
	f := newFixture(t, fixtureOptions{maxAttempts: 3})
	localAuthToken, err := hex.DecodeString(mvpAdminToken)
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewHandlerWithOrchestration(
		f.database, f.projects, nil, f.agents, f.brainService, f.runs, f.tasks, f.conversations,
		localAuthToken, nil, f.service,
	)

	unauthorized := serveMVPRequest(t, handler, http.MethodGet, "/api/v1/projects/"+f.project.ID(), "", false)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated project read status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}
	var denialCount int
	if err := f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM authorization_denials WHERE reason='authentication_required'`).Scan(&denialCount); err != nil {
		t.Fatal(err)
	}
	if denialCount != 1 {
		t.Fatalf("authentication denial audit count = %d, want 1", denialCount)
	}

	projectResponse := serveMVPRequest(t, handler, http.MethodGet, "/api/v1/projects/"+f.project.ID(), "", true)
	if projectResponse.Code != http.StatusOK || !strings.Contains(projectResponse.Body.String(), `"id":"`+f.project.ID()+`"`) {
		t.Fatalf("project response status/body = %d/%s", projectResponse.Code, projectResponse.Body.String())
	}
	brainResponse := serveMVPRequest(t, handler, http.MethodGet, "/api/v1/projects/"+f.project.ID()+"/brain", "", true)
	if brainResponse.Code != http.StatusOK || !strings.Contains(brainResponse.Body.String(), f.brain.AgentID()) {
		t.Fatalf("Brain response status/body = %d/%s", brainResponse.Code, brainResponse.Body.String())
	}
	workerResponse := serveMVPRequest(t, handler, http.MethodGet, "/api/v1/agents/"+f.worker.ID(), "", true)
	if workerResponse.Code != http.StatusOK || !strings.Contains(workerResponse.Body.String(), `"task.transition"`) {
		t.Fatalf("Worker response status/body = %d/%s", workerResponse.Code, workerResponse.Body.String())
	}

	created := serveMVPRequest(t, handler, http.MethodPost, "/api/v1/projects/"+f.project.ID()+"/tasks",
		`{"title":"MVP deliverable","description":"Implement and report the requested change.","reason":"create MVP task"}`, true)
	if created.Code != http.StatusCreated {
		t.Fatalf("task creation status/body = %d/%s", created.Code, created.Body.String())
	}
	var createdTask struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		AssignedTo string `json:"assigned_to"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdTask); err != nil {
		t.Fatal(err)
	}
	if createdTask.ID == "" || createdTask.Status != "BACKLOG" || createdTask.AssignedTo != "" {
		t.Fatalf("created task = %+v", createdTask)
	}

	if _, err := f.tasks.Assign(context.Background(), createdTask.ID, f.worker.ID(), "unauthorized Brain", task.Actor{Type: task.ActorBrain, ID: "not-the-active-brain"}); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("assignment from an unconfigured Brain error = %v, want task.ErrForbidden", err)
	}
	assigned, err := f.tasks.Assign(context.Background(), createdTask.ID, f.worker.ID(), "active Brain assigned eligible Worker", task.Actor{Type: task.ActorBrain, ID: f.brain.AgentID()})
	if err != nil {
		t.Fatalf("active Brain assignment: %v", err)
	}
	if assigned.AssignedTo() != f.worker.ID() {
		t.Fatalf("task assignee = %q, want %q", assigned.AssignedTo(), f.worker.ID())
	}

	ready := serveMVPRequest(t, handler, http.MethodPost, "/api/v1/tasks/"+createdTask.ID+"/transitions", `{"status":"READY","reason":"ready for execution"}`, true)
	if ready.Code != http.StatusOK || !strings.Contains(ready.Body.String(), `"status":"READY"`) {
		t.Fatalf("ready transition status/body = %d/%s", ready.Code, ready.Body.String())
	}
	message := serveMVPRequest(t, handler, http.MethodPost, "/api/v1/tasks/"+createdTask.ID+"/messages",
		`{"content":"Please implement the deliverable and summarize the result.","content_type":"text/plain"}`, true)
	if message.Code != http.StatusCreated {
		t.Fatalf("conversation message status/body = %d/%s", message.Code, message.Body.String())
	}

	dispatched := serveMVPRequest(t, handler, http.MethodPost, "/api/v1/projects/"+f.project.ID()+"/orchestration/dispatch", "", true)
	if dispatched.Code != http.StatusOK {
		t.Fatalf("dispatch status/body = %d/%s", dispatched.Code, dispatched.Body.String())
	}
	var report struct {
		Outcomes []struct {
			TaskID string `json:"task_id"`
			RunID  string `json:"run_id"`
			Status string `json:"status"`
		} `json:"outcomes"`
	}
	if err := json.Unmarshal(dispatched.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].TaskID != createdTask.ID || report.Outcomes[0].RunID == "" || report.Outcomes[0].Status != "SUCCEEDED" {
		t.Fatalf("dispatch report = %+v", report)
	}

	review := serveMVPRequest(t, handler, http.MethodPost, "/api/v1/tasks/"+createdTask.ID+"/review", `{"decision":"APPROVE","reason":"accept fake worker output"}`, true)
	if review.Code != http.StatusOK || !strings.Contains(review.Body.String(), `"status":"DONE"`) {
		t.Fatalf("Brain review status/body = %d/%s", review.Code, review.Body.String())
	}

	taskResponse := serveMVPRequest(t, handler, http.MethodGet, "/api/v1/tasks/"+createdTask.ID, "", true)
	if taskResponse.Code != http.StatusOK {
		t.Fatalf("task history status/body = %d/%s", taskResponse.Code, taskResponse.Body.String())
	}
	var taskHistory struct {
		Status      string `json:"status"`
		AssignedTo  string `json:"assigned_to"`
		AuditEvents []struct {
			EventType string `json:"event_type"`
			ToStatus  string `json:"to_status"`
		} `json:"audit_events"`
	}
	if err := json.Unmarshal(taskResponse.Body.Bytes(), &taskHistory); err != nil {
		t.Fatal(err)
	}
	if taskHistory.Status != "DONE" || taskHistory.AssignedTo != f.worker.ID() {
		t.Fatalf("final task history = %+v", taskHistory)
	}
	seenTaskEvents := map[string]bool{}
	seenStatuses := map[string]bool{}
	for _, event := range taskHistory.AuditEvents {
		seenTaskEvents[event.EventType] = true
		if event.ToStatus != "" {
			seenStatuses[event.ToStatus] = true
		}
	}
	for _, expected := range []string{"task.created", "task.assigned", "task.status.changed"} {
		if !seenTaskEvents[expected] {
			t.Errorf("task audit history missing %s: %+v", expected, taskHistory.AuditEvents)
		}
	}
	for _, expected := range []string{"READY", "WORKING", "REVIEW", "DONE"} {
		if !seenStatuses[expected] {
			t.Errorf("task audit history missing status %s: %+v", expected, taskHistory.AuditEvents)
		}
	}

	runResponse := serveMVPRequest(t, handler, http.MethodGet, "/api/v1/runs/"+report.Outcomes[0].RunID, "", true)
	if runResponse.Code != http.StatusOK || !strings.Contains(runResponse.Body.String(), `"task_id":"`+createdTask.ID+`"`) ||
		!strings.Contains(runResponse.Body.String(), `"agent_id":"`+f.worker.ID()+`"`) || !strings.Contains(runResponse.Body.String(), `"event_type":"run.status.changed"`) {
		t.Fatalf("linked Run history status/body = %d/%s", runResponse.Code, runResponse.Body.String())
	}
	conversationHistory := serveMVPRequest(t, handler, http.MethodGet, "/api/v1/tasks/"+createdTask.ID+"/messages?limit=50", "", true)
	if conversationHistory.Code != http.StatusOK || !strings.Contains(conversationHistory.Body.String(), "Please implement the deliverable") {
		t.Fatalf("conversation history status/body = %d/%s", conversationHistory.Code, conversationHistory.Body.String())
	}
}

func serveMVPRequest(t *testing.T, handler http.Handler, method, path, body string, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if authenticated {
		request.Header.Set("Authorization", "Bearer "+mvpAdminToken)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
