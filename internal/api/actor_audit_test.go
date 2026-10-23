package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/capability"
	"boAIrd/internal/profile"
	"boAIrd/internal/project"
	"boAIrd/internal/storage"
	"boAIrd/internal/task"
	"boAIrd/internal/template"
)

const registeredAdministratorID = "registered-administrator"

func requestWithPrincipal(method, target, body string, principal capability.Principal) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	return request.WithContext(context.WithValue(request.Context(), principalContextKey{}, principal))
}

func TestAgentScopeAuditUsesAuthenticatedPrincipal(t *testing.T) {
	database, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	projectRepository, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projectManager, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	projectValue, err := projectManager.Create(context.Background(), project.CreateInput{Name: "Audit actor"})
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
	agentValue, err := agentManager.Create(context.Background(), agent.CreateInput{Name: "Scoped Agent", Role: "worker", Runtime: agent.RuntimeMCP})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAgentRoutes(mux, agentManager, projectManager, nil)
	principal := capability.Principal{
		ID: registeredAdministratorID, Capabilities: []capability.Capability{"agent.scope.manage"},
		ProjectIDs: []string{projectValue.ID()},
	}
	path := "/api/v1/projects/" + projectValue.ID() + "/agents/" + agentValue.ID()
	for _, testCase := range []struct {
		method string
		event  string
	}{{http.MethodPut, "agent.project.joined"}, {http.MethodDelete, "agent.project.left"}} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, requestWithPrincipal(testCase.method, path, "", principal))
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s scope response = %d, body = %s", testCase.method, response.Code, response.Body.String())
		}
		var actorID string
		if err := database.QueryRow(`SELECT actor_id FROM audit_events WHERE event_type = ? ORDER BY id DESC LIMIT 1`, testCase.event).Scan(&actorID); err != nil {
			t.Fatal(err)
		}
		if actorID != registeredAdministratorID {
			t.Fatalf("%s audit actor = %q, want %q", testCase.event, actorID, registeredAdministratorID)
		}
	}
}

type actorRecordingProfileManager struct{ actorID string }

func (m *actorRecordingProfileManager) Effective(context.Context, string, string) (profile.Effective, error) {
	return profile.Effective{}, nil
}

func (m *actorRecordingProfileManager) ListEffective(context.Context, string) ([]profile.Effective, error) {
	return nil, nil
}

func (m *actorRecordingProfileManager) Save(_ context.Context, _, _ string, input profile.SaveInput) (profile.Effective, error) {
	m.actorID = input.ActorID
	return profile.Effective{}, nil
}

func TestProfileAuditUsesAuthenticatedPrincipal(t *testing.T) {
	manager := &actorRecordingProfileManager{}
	mux := http.NewServeMux()
	registerProfileRoutes(mux, manager)
	request := requestWithPrincipal(http.MethodPut, "/api/v1/projects/project-1/profiles/qa", `{}`, capability.Principal{ID: registeredAdministratorID})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || manager.actorID != registeredAdministratorID {
		t.Fatalf("profile save response = %d, actor = %q, body = %s", response.Code, manager.actorID, response.Body.String())
	}
}

type actorRecordingTemplateManager struct{ actors []string }

func (m *actorRecordingTemplateManager) Create(_ context.Context, _ template.CreateInput, actorID string) (template.Template, error) {
	m.actors = append(m.actors, actorID)
	return template.Template{}, nil
}

func (m *actorRecordingTemplateManager) Get(context.Context, string) (template.Template, error) {
	return template.Template{}, nil
}

func (m *actorRecordingTemplateManager) List(context.Context) ([]template.Template, error) {
	return nil, nil
}

func (m *actorRecordingTemplateManager) Update(_ context.Context, _ string, _ template.UpdateInput, actorID string) (template.Template, error) {
	m.actors = append(m.actors, actorID)
	return template.Template{}, nil
}

func (m *actorRecordingTemplateManager) Instantiate(_ context.Context, _, _ string, _ template.InstantiateInput, actorID string) (agent.Agent, error) {
	m.actors = append(m.actors, actorID)
	return agent.Agent{}, nil
}

func TestTemplateAuditUsesAuthenticatedPrincipal(t *testing.T) {
	manager := &actorRecordingTemplateManager{}
	mux := http.NewServeMux()
	registerTemplateRoutes(mux, manager)
	principal := capability.Principal{
		ID: registeredAdministratorID, Capabilities: []capability.Capability{"agent.create", "agent.update", "agent.scope.manage"},
		ProjectIDs: []string{"project-1"},
	}
	requests := []struct {
		method string
		target string
		body   string
	}{{http.MethodPost, "/api/v1/templates", `{}`},
		{http.MethodPatch, "/api/v1/templates/qa", `{}`},
		{http.MethodPost, "/api/v1/projects/project-1/templates/qa/instantiate", `{}`}}
	for _, testCase := range requests {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, requestWithPrincipal(testCase.method, testCase.target, testCase.body, principal))
		if response.Code < 200 || response.Code >= 300 {
			t.Fatalf("%s %s response = %d, body = %s", testCase.method, testCase.target, response.Code, response.Body.String())
		}
	}
	if len(manager.actors) != len(requests) {
		t.Fatalf("template audit actors = %v, want %d mutations", manager.actors, len(requests))
	}
	for _, actorID := range manager.actors {
		if actorID != registeredAdministratorID {
			t.Fatalf("template audit actor = %q, want %q", actorID, registeredAdministratorID)
		}
	}
}

func TestAuthorizationDenialsDoNotDistinguishProjectScopedResourceExistence(t *testing.T) {
	database, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	projectRepository, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	projectValue, err := projects.Create(context.Background(), project.CreateInput{Name: "Restricted resource"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`DELETE FROM project_members WHERE project_id = ? AND user_id = ?`, projectValue.ID(), project.LocalAdministratorID); err != nil {
		t.Fatal(err)
	}
	knownTask, err := task.New("known", projectValue.ID(), "Known task", "", project.LocalAdministratorID)
	if err != nil {
		t.Fatal(err)
	}
	tasks := taskLookupManager{known: knownTask}
	users := bootstrapTestUserManager{}
	handler := secureAPI(http.NotFoundHandler(), database, projects, &tasks, nil, users, testLocalAuthTokenBytes(), newLocalSessionStore())
	request := func(target string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, target, nil))
		return response
	}
	knownResource := request("/api/v1/tasks/known")
	missingResource := request("/api/v1/tasks/missing")
	if knownResource.Code != missingResource.Code || knownResource.Body.String() != missingResource.Body.String() {
		t.Fatalf("project resource denial differs by existence: known=%d %s missing=%d %s", knownResource.Code, knownResource.Body.String(), missingResource.Code, missingResource.Body.String())
	}
}

type taskLookupManager struct{ known task.Task }

func (m *taskLookupManager) Create(context.Context, string, task.CreateInput, task.Actor) (task.Task, error) {
	return task.Task{}, errors.New("not implemented")
}

func (m *taskLookupManager) Get(_ context.Context, id string) (task.Task, error) {
	if id == "known" {
		return m.known, nil
	}
	return task.Task{}, errors.New("task not found")
}

func (m *taskLookupManager) List(context.Context, string, task.Status, string, int, int) ([]task.Task, error) {
	return nil, errors.New("not implemented")
}

func (m *taskLookupManager) Assign(context.Context, string, string, string, task.Actor) (task.Task, error) {
	return task.Task{}, errors.New("not implemented")
}

func (m *taskLookupManager) Transition(context.Context, string, task.Status, string, task.Actor) (task.Task, error) {
	return task.Task{}, errors.New("not implemented")
}

func (m *taskLookupManager) ListAuditEvents(context.Context, string) ([]task.AuditEvent, error) {
	return nil, errors.New("not implemented")
}
