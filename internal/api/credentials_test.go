package api

import (
	"context"
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

type credentialStub struct{ value credential.Metadata }

func (s credentialStub) Create(_ context.Context, name, provider, _ string) (credential.Metadata, error) {
	return credential.Metadata{ID: "cred-1", Name: name, Provider: provider}, nil
}
func (s credentialStub) List(context.Context) ([]credential.Metadata, error) {
	return []credential.Metadata{s.value}, nil
}
func (s credentialStub) Delete(context.Context, string) error { return nil }

type failedCredentialStub struct{ credentialStub }

func (failedCredentialStub) Create(context.Context, string, string, string) (credential.Metadata, error) {
	return credential.Metadata{}, credential.ErrInvalidInput
}

func TestCredentialEndpointsNeverReturnSubmittedSecret(t *testing.T) {
	secret := "fake-api-key-123456789"
	manager := credentialStub{value: credential.Metadata{ID: "cred-1", Name: "test", Provider: "openai"}}
	handler := newAuthorizedTestHandler(fakeDatabase{}, nil, manager, nil, nil, nil, nil, nil, testLocalAuthTokenBytes())
	request := authorizedTestRequest(http.MethodPost, "/api/v1/credentials", strings.NewReader(`{"name":"test","provider":"openai","secret":"`+secret+`"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secret) || !strings.Contains(response.Body.String(), "cred-1") {
		t.Fatalf("unsafe create response: %s", response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, "/api/v1/credentials", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), secret) {
		t.Fatalf("unsafe list response: %d %s", response.Code, response.Body.String())
	}
}

func TestCredentialEndpointDoesNotEchoInvalidSecret(t *testing.T) {
	handler := newAuthorizedTestHandler(fakeDatabase{}, nil, failedCredentialStub{}, nil, nil, nil, nil, nil, testLocalAuthTokenBytes())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodPost, "/api/v1/credentials", strings.NewReader(`{"name":"x","provider":"openai","secret":"bad-secret"}`)))
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "bad-secret") {
		t.Fatalf("unexpected error response: %d %s", response.Code, response.Body.String())
	}
}

func TestGitHubCredentialBindingRequiresAdminAndIsProjectScoped(t *testing.T) {
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
	projectRepository, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	projectOne, err := projects.Create(ctx, project.CreateInput{Name: "GitHub binding one"})
	if err != nil {
		t.Fatal(err)
	}
	projectTwo, err := projects.Create(ctx, project.CreateInput{Name: "GitHub binding two"})
	if err != nil {
		t.Fatal(err)
	}
	agentRepository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	agents, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := agents.Create(ctx, agent.CreateInput{Name: "GitHub MCP", Role: "worker", Runtime: agent.RuntimeMCP, Capabilities: []string{"github.issue.read"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := agents.AddProject(ctx, worker.ID(), projectOne.ID(), project.LocalAdministratorID); err != nil {
		t.Fatal(err)
	}
	credentialRepository, err := storage.NewCredentialRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := credential.NewManager(credentialRepository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	secret := "ghp_admin_binding_secret_123456"
	githubCredential, err := credentials.Create(ctx, "GitHub token", credential.ProviderGitHub, secret)
	if err != nil {
		t.Fatal(err)
	}
	openAICredential, err := credentials.Create(ctx, "OpenAI key", "openai", "sk-openai-key-for-test-123456")
	if err != nil {
		t.Fatal(err)
	}
	handler := newAuthorizedTestHandler(database, projects, credentials, nil, nil, nil, nil, nil, testLocalAuthTokenBytes())
	path := "/api/v1/projects/" + projectOne.ID() + "/agents/" + worker.ID() + "/github-credential"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodPut, path, strings.NewReader(`{"credential_id":"`+githubCredential.ID+`"}`)))
	if response.Code != http.StatusNoContent || strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), githubCredential.ID) {
		t.Fatalf("admin binding response = %d %q", response.Code, response.Body.String())
	}
	if resolved, err := credentials.ResolveGitHub(ctx, credential.Scope{AgentID: worker.ID(), ProjectID: projectOne.ID()}); err != nil || resolved != secret {
		t.Fatalf("resolved bound token = %q, %v", resolved, err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodPut, path, strings.NewReader(`{"credential_id":"`+openAICredential.ID+`"}`)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("non-GitHub credential binding status = %d body=%s; want 404", response.Code, response.Body.String())
	}
	crossProjectPath := "/api/v1/projects/" + projectTwo.ID() + "/agents/" + worker.ID() + "/github-credential"
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodPut, crossProjectPath, strings.NewReader(`{"credential_id":"`+githubCredential.ID+`"}`)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-project binding status = %d body=%s; want 404", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"credential_id":"`+githubCredential.ID+`"}`)))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated binding status = %d body=%s; want 401", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodDelete, path, nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("binding revoke status = %d body=%s; want 204", response.Code, response.Body.String())
	}
	if _, err := credentials.ResolveGitHub(ctx, credential.Scope{AgentID: worker.ID(), ProjectID: projectOne.ID()}); err != credential.ErrCredentialUnavailable {
		t.Fatalf("ResolveGitHub() after API revoke error = %v, want unavailable", err)
	}
	var auditCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE event_type IN ('credential.integration.bound', 'credential.integration.unbound')`).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("binding audit count = %d, error = %v; want two committed events", auditCount, err)
	}
}
