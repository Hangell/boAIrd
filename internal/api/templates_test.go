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
	"boAIrd/internal/project"
	"boAIrd/internal/storage"
	"boAIrd/internal/template"
)

func newTemplateHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
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
	templateRepository, err := storage.NewTemplateRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	auditRepository, err := storage.NewAuditRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	service, err := template.NewService(templateRepository)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandlerWithProfilesAuditNotificationsAndTemplates(
		fakeDatabase{}, projectManager, nil, agentManager, nil, nil, nil, nil, testLocalAuthTokenBytes(), nil, nil,
		nil, auditRepository, nil, service,
	)

	created := httptest.NewRecorder()
	handler.ServeHTTP(created, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Template target"}`)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create project status = %d, body = %s", created.Code, created.Body.String())
	}
	var value projectResponse
	if err := json.Unmarshal(created.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return handler, value.ID
}

func TestTemplateAPICreateListInstantiate(t *testing.T) {
	handler, projectID := newTemplateHandler(t)

	create := httptest.NewRecorder()
	handler.ServeHTTP(create, authorizedTestRequest(http.MethodPost, "/api/v1/templates",
		strings.NewReader(`{"key":"go-senior","name":"Go Senior","role":"backend","instructions":"write Go","default_runtime":"LOCAL","suggested_tools":["git"],"suggested_capabilities":["task.read","task.review"]}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create template status = %d, body = %s", create.Code, create.Body.String())
	}

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, authorizedTestRequest(http.MethodGet, "/api/v1/templates", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list templates status = %d", list.Code)
	}
	var page templateListResponse
	if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Key != "go-senior" {
		t.Fatalf("templates = %+v", page.Items)
	}

	instantiate := httptest.NewRecorder()
	handler.ServeHTTP(instantiate, authorizedTestRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/templates/go-senior/instantiate",
		strings.NewReader(`{"name":"go worker","capabilities":["task.read"]}`)))
	if instantiate.Code != http.StatusCreated {
		t.Fatalf("instantiate status = %d, body = %s", instantiate.Code, instantiate.Body.String())
	}
	var createdAgent agentResponse
	if err := json.Unmarshal(instantiate.Body.Bytes(), &createdAgent); err != nil {
		t.Fatal(err)
	}
	if len(createdAgent.Capabilities) != 1 || createdAgent.Capabilities[0] != "task.read" {
		t.Fatalf("instantiated capabilities = %v, want only [task.read]", createdAgent.Capabilities)
	}

	audit := httptest.NewRecorder()
	handler.ServeHTTP(audit, authorizedTestRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/audit?limit=100", nil))
	if audit.Code != http.StatusOK {
		t.Fatalf("audit API status = %d, body = %s", audit.Code, audit.Body.String())
	}
	var auditPage struct {
		Items []struct {
			Action string `json:"action"`
			Result string `json:"result"`
		} `json:"items"`
	}
	if err := json.Unmarshal(audit.Body.Bytes(), &auditPage); err != nil {
		t.Fatal(err)
	}
	results := make(map[string]string, len(auditPage.Items))
	for _, item := range auditPage.Items {
		results[item.Action] = item.Result
	}
	for _, action := range []string{"template.instantiated", "agent.project.joined"} {
		if results[action] != "success" {
			t.Errorf("audit result for %s = %q, want success; events=%+v", action, results[action], auditPage.Items)
		}
	}
}

func TestTemplateAPIRejectsInvalidAndUnauthenticated(t *testing.T) {
	handler, projectID := newTemplateHandler(t)
	create := httptest.NewRecorder()
	handler.ServeHTTP(create, authorizedTestRequest(http.MethodPost, "/api/v1/templates",
		strings.NewReader(`{"key":"go-senior","name":"Go Senior","role":"backend","instructions":"write Go","default_runtime":"LOCAL","suggested_tools":["git"],"suggested_capabilities":["task.read"]}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create template status = %d, body = %s", create.Code, create.Body.String())
	}

	badCapability := httptest.NewRecorder()
	handler.ServeHTTP(badCapability, authorizedTestRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/templates/go-senior/instantiate",
		strings.NewReader(`{"capabilities":["Not-A-Cap"]}`)))
	if badCapability.Code != http.StatusBadRequest {
		t.Fatalf("bad capability status = %d, want 400", badCapability.Code)
	}

	unknown := httptest.NewRecorder()
	handler.ServeHTTP(unknown, authorizedTestRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/templates/unknown/instantiate", strings.NewReader(`{}`)))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown template status = %d, want 404", unknown.Code)
	}

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "/api/v1/templates", strings.NewReader(`{"key":"x","name":"X","role":"r","default_runtime":"LOCAL"}`)))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", unauthenticated.Code)
	}
}
