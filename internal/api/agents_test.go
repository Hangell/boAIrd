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
	"boAIrd/internal/credential"
	"boAIrd/internal/storage"
)

func TestAgentAPIUsesDistinctProvidersAndDoesNotGrantByRole(t *testing.T) {
	database, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	credentialRepository, err := storage.NewCredentialRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	credentialManager, err := credential.NewManager(credentialRepository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	openAIKey, err := credentialManager.Create(context.Background(), "OpenAI", "openai", "fake-openai-key-0001")
	if err != nil {
		t.Fatal(err)
	}
	claudeKey, err := credentialManager.Create(context.Background(), "Anthropic", "anthropic", "fake-anthropic-key-0002")
	if err != nil {
		t.Fatal(err)
	}
	agentRepository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	agentManager, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandlerWithAgents(fakeDatabase{}, nil, credentialManager, agentManager)
	create := func(name, role, provider, model, credentialID string) agentResponse {
		t.Helper()
		body, err := json.Marshal(agent.CreateInput{Name: name, Role: role, Runtime: agent.RuntimeAPI, Provider: provider, Model: model, CredentialID: credentialID})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(string(body))))
		if response.Code != http.StatusCreated {
			t.Fatalf("create status = %d, body=%s", response.Code, response.Body.String())
		}
		var value agentResponse
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		if value.Status != agent.StatusIdle {
			t.Fatalf("initial status = %q", value.Status)
		}
		return value
	}
	backend := create("Backend", "administrator", "openai", "gpt-example", openAIKey.ID)
	qa := create("QA", "administrator", "anthropic", "claude-example", claudeKey.ID)
	if backend.Provider != "openai" || qa.Provider != "anthropic" || backend.Model == qa.Model {
		t.Fatalf("agents do not retain distinct provider configurations: %+v %+v", backend, qa)
	}
	if len(backend.Capabilities) != 0 || len(qa.Capabilities) != 0 {
		t.Fatalf("role granted capabilities: backend=%v qa=%v", backend.Capabilities, qa.Capabilities)
	}
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/agents?limit=10", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"provider":"anthropic"`) {
		t.Fatalf("list status = %d, body=%s", list.Code, list.Body.String())
	}
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+backend.ID, nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), backend.ID) {
		t.Fatalf("get status = %d, body=%s", get.Code, get.Body.String())
	}
	name := "Backend Specialist"
	patch, _ := json.Marshal(agent.UpdateInput{Name: &name})
	updated := httptest.NewRecorder()
	handler.ServeHTTP(updated, httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+backend.ID, strings.NewReader(string(patch))))
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), name) {
		t.Fatalf("patch status = %d, body=%s", updated.Code, updated.Body.String())
	}
	localRuntime := agent.RuntimeLocal
	switchRuntime, _ := json.Marshal(agent.UpdateInput{Runtime: &localRuntime})
	switched := httptest.NewRecorder()
	handler.ServeHTTP(switched, httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+backend.ID, strings.NewReader(string(switchRuntime))))
	if switched.Code != http.StatusOK || strings.Contains(switched.Body.String(), "credential_id") || strings.Contains(switched.Body.String(), `"provider"`) {
		t.Fatalf("switching away from API retained provider configuration: %d %s", switched.Code, switched.Body.String())
	}
}

func TestAgentAPIRejectsRawSecretsAndWritableStatus(t *testing.T) {
	database, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := agent.NewManager(repository)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandlerWithAgents(fakeDatabase{}, nil, nil, manager)
	badSecret := httptest.NewRecorder()
	handler.ServeHTTP(badSecret, httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"x","role":"worker","runtime":"LOCAL","api_key":"do-not-echo-this"}`)))
	if badSecret.Code != http.StatusBadRequest || strings.Contains(badSecret.Body.String(), "do-not-echo-this") {
		t.Fatalf("raw credential field was accepted or echoed: %d %s", badSecret.Code, badSecret.Body.String())
	}
	status := httptest.NewRecorder()
	handler.ServeHTTP(status, httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"x","role":"worker","runtime":"LOCAL","status":"ONLINE"}`)))
	if status.Code != http.StatusBadRequest {
		t.Fatalf("client writable status accepted: %d %s", status.Code, status.Body.String())
	}
}
