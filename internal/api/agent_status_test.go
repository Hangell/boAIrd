package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/heartbeat"
	"boAIrd/internal/storage"
)

type observedAgentManager struct {
	*agent.Manager
	observed map[string]agent.Observed
	events   []heartbeat.StatusEvent
}

func (m *observedAgentManager) Observe(_ context.Context, agentIDs []string) (map[string]agent.Observed, error) {
	values := make(map[string]agent.Observed, len(agentIDs))
	for _, id := range agentIDs {
		if value, ok := m.observed[id]; ok {
			values[id] = value
		}
	}
	return values, nil
}

func (m *observedAgentManager) Events(_ context.Context, agentID string, limit int) ([]heartbeat.StatusEvent, error) {
	if limit < 1 || limit > 100 {
		return nil, heartbeat.ErrInvalidInput
	}
	values := make([]heartbeat.StatusEvent, 0, len(m.events))
	for _, value := range m.events {
		if value.AgentID == agentID {
			values = append(values, value)
		}
	}
	return values, nil
}

func TestAgentAPIReportsObservedStatusAndEvents(t *testing.T) {
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	repository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := agent.NewManager(repository)
	if err != nil {
		t.Fatal(err)
	}
	value, err := manager.Create(ctx, agent.CreateInput{Name: "External Agent", Role: "worker", Runtime: agent.RuntimeMCP})
	if err != nil {
		t.Fatal(err)
	}
	lastSeen := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	observed := &observedAgentManager{
		Manager:  manager,
		observed: map[string]agent.Observed{value.ID(): {Status: agent.StatusBusy, LastSeenAt: &lastSeen, SessionOpen: true}},
		events: []heartbeat.StatusEvent{{
			Sequence: 3, AgentID: value.ID(), SessionID: "session-1",
			From: agent.StatusOnline, To: agent.StatusBusy, Reason: heartbeat.ReasonWorkChanged, CreatedAt: lastSeen,
		}},
	}
	handler := newHandler(fakeDatabase{}, nil, nil, observed, nil, nil, nil, nil, testLocalAuthTokenBytes())

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, "/api/v1/agents/"+value.ID(), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("get status = %d, body=%s", response.Code, response.Body.String())
	}
	var agentValue agentResponse
	if err := json.Unmarshal(response.Body.Bytes(), &agentValue); err != nil {
		t.Fatal(err)
	}
	if agentValue.Status != agent.StatusBusy || !agentValue.SessionOpen || agentValue.LastSeenAt == nil || !agentValue.LastSeenAt.Equal(lastSeen) {
		t.Fatalf("agent response = %+v, want observed BUSY session", agentValue)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, "/api/v1/agents", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", response.Code, response.Body.String())
	}
	var list agentListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Status != agent.StatusBusy {
		t.Fatalf("agent list = %+v, want observed BUSY status", list.Items)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, "/api/v1/agents/"+value.ID()+"/status-events?limit=10", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status events status = %d, body=%s", response.Code, response.Body.String())
	}
	var events agentStatusEventPage
	if err := json.Unmarshal(response.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if len(events.Items) != 1 || events.Items[0].From != agent.StatusOnline || events.Items[0].To != agent.StatusBusy || events.Items[0].Reason != heartbeat.ReasonWorkChanged {
		t.Fatalf("status events = %+v", events.Items)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, "/api/v1/agents/"+value.ID()+"/status-events?limit=500", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, want 400", response.Code)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, "/api/v1/agents/missing/status-events", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing agent status = %d, want 404", response.Code)
	}
}

func TestAgentAPIWithoutObserverKeepsConfiguredStatus(t *testing.T) {
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	repository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := agent.NewManager(repository)
	if err != nil {
		t.Fatal(err)
	}
	value, err := manager.Create(ctx, agent.CreateInput{Name: "External Agent", Role: "worker", Runtime: agent.RuntimeMCP})
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandler(fakeDatabase{}, nil, nil, manager, nil, nil, nil, nil, testLocalAuthTokenBytes())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, "/api/v1/agents/"+value.ID(), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("get status = %d, body=%s", response.Code, response.Body.String())
	}
	var agentValue agentResponse
	if err := json.Unmarshal(response.Body.Bytes(), &agentValue); err != nil {
		t.Fatal(err)
	}
	if agentValue.Status != agent.StatusIdle || agentValue.SessionOpen {
		t.Fatalf("agent response = %+v, want configured IDLE without a session", agentValue)
	}
}
