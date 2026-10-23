package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/project"
	"boAIrd/internal/storage"
	"boAIrd/internal/user"
)

func newUserHandler(t *testing.T) (http.Handler, *sql.DB) {
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
	handler := NewHandlerWithSecurityNotificationsTemplatesAndUsers(
		database, projectManager, nil, nil, nil, nil, nil, nil, testLocalAuthTokenBytes(), nil, nil,
		nil, nil, nil, nil, userService, SecurityPolicy{},
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
	del := httptest.NewRecorder()
	handler.ServeHTTP(del, authorizedTestRequest(http.MethodDelete, "/api/v1/users/alice", nil))
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete user status = %d, body = %s", del.Code, del.Body.String())
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
}

func TestUserAPIMembershipIsolation(t *testing.T) {
	handler, _ := newUserHandler(t)
	projectOne := createTestProject(t, handler)
	projectTwo := createTestProject(t, handler)
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
	if revoked.Code != http.StatusForbidden {
		t.Fatalf("revoked member read project status = %d, want 403", revoked.Code)
	}
}
