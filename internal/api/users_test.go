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
	"boAIrd/internal/user"
)

func newUserHandler(t *testing.T) (http.Handler, *storage.Database) {
	t.Helper()
	database, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
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
	userRepository, err := storage.NewUserRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	userService, err := user.NewService(userRepository, projectRepository)
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
	auditRepository, err := storage.NewAuditRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandlerWithSecurityNotificationsTemplatesAndUsers(
		database, projectManager, nil, agentManager, nil, nil, nil, nil, testLocalAuthTokenBytes(), nil, nil,
		nil, auditRepository, nil, nil, userService, SecurityPolicy{},
	)
	return handler, database
}

func createTestProject(t *testing.T, handler http.Handler) string {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Alpha"}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("create project status = %d, body = %s", response.Code, response.Body.String())
	}
	var value projectResponse
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value.ID
}

func memberSessionCookie(t *testing.T, handler http.Handler, userID string) *http.Cookie {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodPost, "/api/v1/users/"+userID+"/sessions", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("issue member session status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == localAuthCookieName {
			return cookie
		}
	}
	t.Fatal("member session cookie was not set")
	return nil
}

func memberRequest(method, target string, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.AddCookie(cookie)
	return request
}

func TestUserAPIAdministratorCRUD(t *testing.T) {
	handler, _ := newUserHandler(t)
	create := httptest.NewRecorder()
	handler.ServeHTTP(create, authorizedTestRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"id":"alice","name":"Alice"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create user status = %d, body = %s", create.Code, create.Body.String())
	}
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, authorizedTestRequest(http.MethodGet, "/api/v1/users", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"alice"`) {
		t.Fatalf("list users status = %d, body = %s", list.Code, list.Body.String())
	}
	cookie := memberSessionCookie(t, handler, "alice")
	audit := httptest.NewRecorder()
	handler.ServeHTTP(audit, authorizedTestRequest(http.MethodGet, "/api/v1/audit/users", nil))
	if audit.Code != http.StatusOK || !strings.Contains(audit.Body.String(), `"event_type":"user.session.issued"`) || !strings.Contains(audit.Body.String(), `"event_type":"user.created"`) {
		t.Fatalf("user authority audit status = %d, body = %s", audit.Code, audit.Body.String())
	}
	del := httptest.NewRecorder()
	handler.ServeHTTP(del, authorizedTestRequest(http.MethodDelete, "/api/v1/users/alice", nil))
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete user status = %d, body = %s", del.Code, del.Body.String())
	}
	revoked := httptest.NewRecorder()
	handler.ServeHTTP(revoked, memberRequest(http.MethodGet, "/api/v1/session", cookie))
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("deleted user session status = %d, want 401", revoked.Code)
	}
}

func TestAdditionalAdministratorRequiresMembershipForProjectOperations(t *testing.T) {
	handler, _ := newUserHandler(t)
	projectID := createTestProject(t, handler)
	createAdmin := httptest.NewRecorder()
	handler.ServeHTTP(createAdmin, authorizedTestRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"id":"project-admin","name":"Project Admin","role":"administrator"}`)))
	if createAdmin.Code != http.StatusCreated {
		t.Fatalf("create administrator status = %d, body = %s", createAdmin.Code, createAdmin.Body.String())
	}
	createMember := httptest.NewRecorder()
	handler.ServeHTTP(createMember, authorizedTestRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"id":"project-member","name":"Project Member"}`)))
	if createMember.Code != http.StatusCreated {
		t.Fatalf("create member status = %d, body = %s", createMember.Code, createMember.Body.String())
	}
	cookie := memberSessionCookie(t, handler, "project-admin")
	projectList := httptest.NewRecorder()
	handler.ServeHTTP(projectList, memberRequest(http.MethodGet, "/api/v1/projects", cookie))
	if projectList.Code != http.StatusOK || !strings.Contains(projectList.Body.String(), projectID) {
		t.Fatalf("administrator global project list status = %d, body = %s", projectList.Code, projectList.Body.String())
	}
	projectRead := httptest.NewRecorder()
	handler.ServeHTTP(projectRead, memberRequest(http.MethodGet, "/api/v1/projects/"+projectID, cookie))
	if projectRead.Code != http.StatusForbidden {
		t.Fatalf("unscoped administrator project detail status = %d, body = %s", projectRead.Code, projectRead.Body.String())
	}

	grantBootstrap := httptest.NewRecorder()
	grantBootstrapRequest := authorizedTestRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/members/project-admin", strings.NewReader(`{"role":"ADMIN"}`))
	grantBootstrapRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(grantBootstrap, grantBootstrapRequest)
	if grantBootstrap.Code != http.StatusNoContent {
		t.Fatalf("bootstrap administrator membership grant status = %d, body = %s", grantBootstrap.Code, grantBootstrap.Body.String())
	}
	projectRead = httptest.NewRecorder()
	handler.ServeHTTP(projectRead, memberRequest(http.MethodGet, "/api/v1/projects/"+projectID, cookie))
	if projectRead.Code != http.StatusOK {
		t.Fatalf("scoped administrator project detail status = %d, body = %s", projectRead.Code, projectRead.Body.String())
	}

	grantRequest := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/members/project-member", strings.NewReader(`{"role":"MEMBER"}`))
	grantRequest.AddCookie(cookie)
	grantRequest.Header.Set("Content-Type", "application/json")
	grantRequest.Header.Set("Origin", "http://localhost:4200")
	grant := httptest.NewRecorder()
	handler.ServeHTTP(grant, grantRequest)
	if grant.Code != http.StatusNoContent {
		t.Fatalf("administrator membership grant status = %d, body = %s", grant.Code, grant.Body.String())
	}
}

func TestUserAPIProtectsBootstrapAdministrator(t *testing.T) {
	handler, _ := newUserHandler(t)
	del := httptest.NewRecorder()
	handler.ServeHTTP(del, authorizedTestRequest(http.MethodDelete, "/api/v1/users/local-administrator", nil))
	if del.Code != http.StatusForbidden {
		t.Fatalf("delete bootstrap administrator status = %d, want 403", del.Code)
	}
}

func TestUserAPIMemberCannotManageUsers(t *testing.T) {
	handler, _ := newUserHandler(t)
	create := httptest.NewRecorder()
	handler.ServeHTTP(create, authorizedTestRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"id":"alice","name":"Alice"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create user status = %d, body = %s", create.Code, create.Body.String())
	}
	cookie := memberSessionCookie(t, handler, "alice")
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, memberRequest(http.MethodPost, "/api/v1/users", cookie))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("member create user status = %d, want 403", denied.Code)
	}
	audit := httptest.NewRecorder()
	handler.ServeHTTP(audit, memberRequest(http.MethodGet, "/api/v1/audit/users", cookie))
	if audit.Code != http.StatusForbidden {
		t.Fatalf("member user authority audit status = %d, want 403", audit.Code)
	}
}

func TestUserSessionIssuanceFailsClosedWhenAuditFails(t *testing.T) {
	handler, database := newUserHandler(t)
	if _, err := database.Exec(`CREATE TRIGGER fail_user_session_audit BEFORE INSERT ON user_authority_audit_events
		WHEN NEW.event_type = 'user.session.issued' BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodPost, "/api/v1/session", nil))
	if response.Code != http.StatusServiceUnavailable || len(response.Result().Cookies()) != 0 {
		t.Fatalf("session issuance with failed audit status = %d, cookies = %+v, body = %s", response.Code, response.Result().Cookies(), response.Body.String())
	}
}

func TestUserAPIMembershipIsolation(t *testing.T) {
	handler, _ := newUserHandler(t)
	projectOne := createTestProject(t, handler)
	projectTwo := createTestProject(t, handler)
	for _, change := range []struct{ id, name string }{{projectOne, "Zulu"}, {projectTwo, "Alpha"}} {
		body, err := json.Marshal(map[string]string{"name": change.name})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authorizedTestRequest(http.MethodPatch, "/api/v1/projects/"+change.id, strings.NewReader(string(body))))
		if response.Code != http.StatusOK {
			t.Fatalf("rename project status = %d, body = %s", response.Code, response.Body.String())
		}
	}
	create := httptest.NewRecorder()
	handler.ServeHTTP(create, authorizedTestRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"id":"alice","name":"Alice"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create user status = %d", create.Code)
	}
	grant := httptest.NewRecorder()
	handler.ServeHTTP(grant, authorizedTestRequest(http.MethodPut, "/api/v1/projects/"+projectOne+"/members/alice", strings.NewReader(`{"role":"MEMBER"}`)))
	if grant.Code != http.StatusNoContent {
		t.Fatalf("grant membership status = %d, body = %s", grant.Code, grant.Body.String())
	}
	cookie := memberSessionCookie(t, handler, "alice")
	projectList := httptest.NewRecorder()
	handler.ServeHTTP(projectList, memberRequest(http.MethodGet, "/api/v1/projects?limit=1", cookie))
	if projectList.Code != http.StatusOK || !strings.Contains(projectList.Body.String(), projectOne) || strings.Contains(projectList.Body.String(), projectTwo) {
		t.Fatalf("member project list status = %d, body = %s", projectList.Code, projectList.Body.String())
	}
	allowed := httptest.NewRecorder()
	handler.ServeHTTP(allowed, memberRequest(http.MethodGet, "/api/v1/projects/"+projectOne, cookie))
	if allowed.Code != http.StatusOK {
		t.Fatalf("member read own project status = %d, body = %s", allowed.Code, allowed.Body.String())
	}
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, memberRequest(http.MethodGet, "/api/v1/projects/"+projectTwo, cookie))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("member read foreign project status = %d, want 403", denied.Code)
	}
	revoke := httptest.NewRecorder()
	handler.ServeHTTP(revoke, authorizedTestRequest(http.MethodDelete, "/api/v1/projects/"+projectOne+"/members/alice", nil))
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("revoke membership status = %d, body = %s", revoke.Code, revoke.Body.String())
	}
	revoked := httptest.NewRecorder()
	handler.ServeHTTP(revoked, memberRequest(http.MethodGet, "/api/v1/projects/"+projectOne, cookie))
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked member read project status = %d, want 401", revoked.Code)
	}
}

func TestMemberAgentReadsAreProjectScoped(t *testing.T) {
	handler, _ := newUserHandler(t)
	projectOne := createTestProject(t, handler)
	projectTwo := createTestProject(t, handler)
	create := httptest.NewRecorder()
	handler.ServeHTTP(create, authorizedTestRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"id":"alice","name":"Alice"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create user status = %d, body = %s", create.Code, create.Body.String())
	}
	grant := httptest.NewRecorder()
	handler.ServeHTTP(grant, authorizedTestRequest(http.MethodPut, "/api/v1/projects/"+projectOne+"/members/alice", strings.NewReader(`{"role":"MEMBER"}`)))
	if grant.Code != http.StatusNoContent {
		t.Fatalf("grant membership status = %d, body = %s", grant.Code, grant.Body.String())
	}
	createAgent := func(name, instructions string) agentResponse {
		t.Helper()
		body, err := json.Marshal(agent.CreateInput{Name: name, Role: "worker", Runtime: agent.RuntimeMCP, Instructions: instructions, Capabilities: []string{"task.read"}})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authorizedTestRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(string(body))))
		if response.Code != http.StatusCreated {
			t.Fatalf("create agent status = %d, body = %s", response.Code, response.Body.String())
		}
		var value agentResponse
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	addToProject := func(projectID, agentID string) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authorizedTestRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/agents/"+agentID, nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("scope agent status = %d, body = %s", response.Code, response.Body.String())
		}
	}
	visible := createAgent("Visible", "member-visible-instructions")
	foreign := createAgent("Foreign", "must-not-leak-foreign-instructions")
	addToProject(projectOne, visible.ID)
	addToProject(projectTwo, foreign.ID)
	cookie := memberSessionCookie(t, handler, "alice")

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, memberRequest(http.MethodGet, "/api/v1/agents?limit=1", cookie))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), visible.ID) || strings.Contains(list.Body.String(), foreign.ID) || strings.Contains(list.Body.String(), "must-not-leak-foreign-instructions") {
		t.Fatalf("member agent list status = %d, body = %s", list.Code, list.Body.String())
	}
	getVisible := httptest.NewRecorder()
	handler.ServeHTTP(getVisible, memberRequest(http.MethodGet, "/api/v1/agents/"+visible.ID, cookie))
	if getVisible.Code != http.StatusOK || !strings.Contains(getVisible.Body.String(), projectOne) {
		t.Fatalf("member get own agent status = %d, body = %s", getVisible.Code, getVisible.Body.String())
	}
	for _, path := range []string{"/api/v1/agents/" + foreign.ID, "/api/v1/agents/" + foreign.ID + "/status-events"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, memberRequest(http.MethodGet, path, cookie))
		if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "must-not-leak-foreign-instructions") {
			t.Errorf("out-of-project agent read %s status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}
}
