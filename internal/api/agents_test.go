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
	"boAIrd/internal/project"
	"boAIrd/internal/storage"
)

func TestAgentAPIUsesDistinctProvidersAndDoesNotGrantByRole(t *testing.T) {
	database, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
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
	handler := newAuthorizedTestHandler(fakeDatabase{}, nil, credentialManager, agentManager, nil, nil, nil, nil, testLocalAuthTokenBytes())
	create := func(name, role, provider, model, credentialID string) agentResponse {
		t.Helper()
		body, err := json.Marshal(agent.CreateInput{Name: name, Role: role, Runtime: agent.RuntimeAPI, Provider: provider, Model: model, CredentialID: credentialID})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authorizedTestRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(string(body))))
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
	handler.ServeHTTP(list, authorizedTestRequest(http.MethodGet, "/api/v1/agents?limit=10", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"provider":"anthropic"`) {
		t.Fatalf("list status = %d, body=%s", list.Code, list.Body.String())
	}
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, authorizedTestRequest(http.MethodGet, "/api/v1/agents/"+backend.ID, nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), backend.ID) {
		t.Fatalf("get status = %d, body=%s", get.Code, get.Body.String())
	}
	name := "Backend Specialist"
	patch, _ := json.Marshal(agent.UpdateInput{Name: &name})
	updated := httptest.NewRecorder()
	handler.ServeHTTP(updated, authorizedTestRequest(http.MethodPatch, "/api/v1/agents/"+backend.ID, strings.NewReader(string(patch))))
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), name) {
		t.Fatalf("patch status = %d, body=%s", updated.Code, updated.Body.String())
	}
	localRuntime := agent.RuntimeLocal
	switchRuntime, _ := json.Marshal(agent.UpdateInput{Runtime: &localRuntime})
	switched := httptest.NewRecorder()
	handler.ServeHTTP(switched, authorizedTestRequest(http.MethodPatch, "/api/v1/agents/"+backend.ID, strings.NewReader(string(switchRuntime))))
	if switched.Code != http.StatusOK || strings.Contains(switched.Body.String(), "credential_id") || strings.Contains(switched.Body.String(), `"provider"`) {
		t.Fatalf("switching away from API retained provider configuration: %d %s", switched.Code, switched.Body.String())
	}
}

func TestAgentProjectScopeRoutesRequireAdministratorProjectMembership(t *testing.T) {
	database, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	projectRepository, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projectManager, err := project.NewManager(projectRepository)
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
	value, err := agentManager.Create(context.Background(), agent.CreateInput{Name: "Scoped Agent", Role: "worker", Runtime: agent.RuntimeMCP})
	if err != nil {
		t.Fatal(err)
	}
	handler := newAuthorizedTestHandler(fakeDatabase{}, projectManager, nil, agentManager, nil, nil, nil, nil, testLocalAuthTokenBytes())
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Scope target"}`)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create project status = %d, body = %s", created.Code, created.Body.String())
	}
	var projectValue projectResponse
	if err := json.Unmarshal(created.Body.Bytes(), &projectValue); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/projects/" + projectValue.ID + "/agents/" + value.ID()
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPut, path, nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated scope grant status = %d, want 401", unauthenticated.Code)
	}
	granted := httptest.NewRecorder()
	handler.ServeHTTP(granted, authorizedTestRequest(http.MethodPut, path, nil))
	if granted.Code != http.StatusNoContent {
		t.Fatalf("scope grant status = %d, body = %s", granted.Code, granted.Body.String())
	}
	loaded, err := agentManager.Get(context.Background(), value.ID())
	if err != nil || len(loaded.ProjectIDs()) != 1 || loaded.ProjectIDs()[0] != projectValue.ID {
		t.Fatalf("Agent scope after grant = %v, error = %v", loaded.ProjectIDs(), err)
	}
	revoked := httptest.NewRecorder()
	handler.ServeHTTP(revoked, authorizedTestRequest(http.MethodDelete, path, nil))
	if revoked.Code != http.StatusNoContent {
		t.Fatalf("scope revoke status = %d, body = %s", revoked.Code, revoked.Body.String())
	}
	unknownAgent := httptest.NewRecorder()
	handler.ServeHTTP(unknownAgent, authorizedTestRequest(http.MethodPut, "/api/v1/projects/"+projectValue.ID+"/agents/missing-agent", nil))
	if unknownAgent.Code != http.StatusNotFound {
		t.Fatalf("unknown Agent scope grant status = %d, want 404", unknownAgent.Code)
	}
	unknownProject := httptest.NewRecorder()
	handler.ServeHTTP(unknownProject, authorizedTestRequest(http.MethodPut, "/api/v1/projects/missing-project/agents/"+value.ID(), nil))
	if unknownProject.Code != http.StatusForbidden {
		t.Fatalf("out-of-scope project grant status = %d, want 403", unknownProject.Code)
	}
	if err := agentManager.AddProject(context.Background(), value.ID(), projectValue.ID, project.LocalAdministratorID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`DELETE FROM project_members WHERE project_id = ? AND user_id = 'local-administrator'`, projectValue.ID); err != nil {
		t.Fatal(err)
	}
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, authorizedTestRequest(http.MethodDelete, path, nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("non-member scope revoke status = %d, want 403; body = %s", denied.Code, denied.Body.String())
	}
	if capability, _, ok := requiredAPICapability(httptest.NewRequest(http.MethodPut, path, nil), projectManager, nil, nil); !ok || capability != "agent.scope.manage" {
		t.Fatalf("scope route capability = %q, %v, want agent.scope.manage", capability, ok)
	}
}

func TestAgentAPIRejectsRawSecretsAndWritableStatus(t *testing.T) {
	database, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
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
	handler := newAuthorizedTestHandler(fakeDatabase{}, nil, nil, manager, nil, nil, nil, nil, testLocalAuthTokenBytes())
	badSecret := httptest.NewRecorder()
	handler.ServeHTTP(badSecret, authorizedTestRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"x","role":"worker","runtime":"LOCAL","api_key":"do-not-echo-this"}`)))
	if badSecret.Code != http.StatusBadRequest || strings.Contains(badSecret.Body.String(), "do-not-echo-this") {
		t.Fatalf("raw credential field was accepted or echoed: %d %s", badSecret.Code, badSecret.Body.String())
	}
	status := httptest.NewRecorder()
	handler.ServeHTTP(status, authorizedTestRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"x","role":"worker","runtime":"LOCAL","status":"ONLINE"}`)))
	if status.Code != http.StatusBadRequest {
		t.Fatalf("client writable status accepted: %d %s", status.Code, status.Body.String())
	}
}
