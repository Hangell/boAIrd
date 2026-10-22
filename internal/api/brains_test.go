package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boAIrd/internal/brain"
)

type brainRouteStub struct {
	actorID    string
	reason     string
	executeErr error
}

func (s *brainRouteStub) Create(context.Context, string, brain.CreateInput) (brain.Brain, error) {
	return brain.New("brain-1", "project-1", "agent-1", brain.AutonomyManual)
}

func (s *brainRouteStub) Get(context.Context, string) (brain.Brain, error) {
	return brain.New("brain-1", "project-1", "agent-1", brain.AutonomyManual)
}

func (s *brainRouteStub) Execute(context.Context, string, string, int) (brain.Execution, error) {
	return brain.Execution{}, s.executeErr
}

func TestExternalBrainRunEndpointReturnsConflict(t *testing.T) {
	manager := &brainRouteStub{executeErr: brain.ErrExternalRuntime}
	mux := http.NewServeMux()
	registerBrainRoutes(mux, manager, testConversationToken(t), newLocalSessionStore())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/brain/runs", strings.NewReader(`{"objective":"plan"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"external_runtime"`) {
		t.Fatalf("external Brain run status = %d, body = %s", response.Code, response.Body.String())
	}
}

func (s *brainRouteStub) Replace(_ context.Context, _ string, input brain.CreateInput, actorID, reason string) (brain.Brain, error) {
	s.actorID, s.reason = actorID, reason
	return brain.New("brain-1", "project-1", input.AgentID, input.Autonomy)
}

func TestBrainReplacementRequiresLocalAdministratorToken(t *testing.T) {
	manager := &brainRouteStub{}
	mux := http.NewServeMux()
	registerBrainRoutes(mux, manager, testConversationToken(t), newLocalSessionStore())
	request := httptest.NewRequest(http.MethodPut, "/api/v1/projects/project-1/brain", strings.NewReader(`{"agent_id":"agent-2","autonomy":"MANUAL","reason":"replace agent"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized replacement status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if manager.actorID != "" {
		t.Fatalf("unauthorized request reached manager as %q", manager.actorID)
	}
}

func TestBrainReplacementUsesAuthenticatedPrincipalAndReason(t *testing.T) {
	manager := &brainRouteStub{}
	mux := http.NewServeMux()
	registerBrainRoutes(mux, manager, testConversationToken(t), newLocalSessionStore())
	request := httptest.NewRequest(http.MethodPut, "/api/v1/projects/project-1/brain", strings.NewReader(`{"agent_id":"agent-2","autonomy":"MANUAL","reason":"replace agent"}`))
	request.Header.Set("Authorization", "Bearer "+testConversationTokenHex)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authorized replacement status = %d, body = %s", response.Code, response.Body.String())
	}
	if manager.actorID != "local-administrator" || manager.reason != "replace agent" {
		t.Fatalf("replacement actor=%q reason=%q", manager.actorID, manager.reason)
	}
}
