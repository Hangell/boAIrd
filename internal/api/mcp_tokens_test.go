package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/mcptoken"
	"boAIrd/internal/storage"
)

func TestMCPTokenAdministrationRequiresAdminAndReturnsSecretOnce(t *testing.T) {
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	agentRepository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	value, err := agent.NewConfigured("agent-1", agent.Configuration{Name: "External Agent", Role: "worker", Runtime: agent.RuntimeMCP})
	if err != nil {
		t.Fatal(err)
	}
	if err := agentRepository.Create(ctx, value); err != nil {
		t.Fatal(err)
	}
	agentManager, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	tokenRepository, err := storage.NewMCPTokenRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	tokenManager, err := mcptoken.NewService(tokenRepository, agentManager)
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandlerWithMCP(database, nil, nil, agentManager, nil, nil, nil, nil, testLocalAuthTokenBytes(), tokenManager)
	invalidLifetime := httptest.NewRecorder()
	handler.ServeHTTP(invalidLifetime, authorizedTestRequest(http.MethodPost, "/api/v1/mcp/tokens", strings.NewReader(`{"agent_id":"agent-1","expires_in_seconds":31536001}`)))
	if invalidLifetime.Code != http.StatusBadRequest {
		t.Fatalf("invalid lifetime status = %d, body = %s", invalidLifetime.Code, invalidLifetime.Body.String())
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/mcp/tokens", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated token list status = %d", unauthorized.Code)
	}

	body := `{"agent_id":"agent-1","expires_in_seconds":3600}`
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, authorizedTestRequest(http.MethodPost, "/api/v1/mcp/tokens", strings.NewReader(body)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create token status = %d, body = %s", created.Code, created.Body.String())
	}
	var first mcpTokenIssuedResponse
	if err := json.Unmarshal(created.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Token) != 64 || first.AgentID != value.ID() || first.Status != "active" {
		t.Fatalf("issued token response = %+v", first)
	}

	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, authorizedTestRequest(http.MethodGet, "/api/v1/mcp/tokens", nil))
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), first.Token) || strings.Contains(listed.Body.String(), "token_hash") {
		t.Fatalf("token listing leaked secret or failed: %d %s", listed.Code, listed.Body.String())
	}

	rotateBody := `{"expires_in_seconds":7200}`
	rotated := httptest.NewRecorder()
	handler.ServeHTTP(rotated, authorizedTestRequest(http.MethodPost, "/api/v1/mcp/tokens/"+first.ID+"/rotate", strings.NewReader(rotateBody)))
	if rotated.Code != http.StatusCreated {
		t.Fatalf("rotate status = %d, body = %s", rotated.Code, rotated.Body.String())
	}
	var second mcpTokenIssuedResponse
	if err := json.Unmarshal(rotated.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.AgentID != first.AgentID || second.Token == first.Token {
		t.Fatalf("rotated token response = %+v", second)
	}
	if _, err := tokenManager.Verify(ctx, first.Token); err == nil {
		t.Fatal("rotated token remained valid")
	}

	revoked := httptest.NewRecorder()
	handler.ServeHTTP(revoked, authorizedTestRequest(http.MethodDelete, "/api/v1/mcp/tokens/"+second.ID, nil))
	if revoked.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d, body = %s", revoked.Code, revoked.Body.String())
	}
	if _, err := tokenManager.Verify(ctx, second.Token); err == nil {
		t.Fatal("revoked token remained valid")
	}
	var events int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM mcp_agent_token_events WHERE actor_id = 'local-administrator'`).Scan(&events); err != nil || events != 3 {
		t.Fatalf("administrator token audit events = %d, error = %v", events, err)
	}
}
