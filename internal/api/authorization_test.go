package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRESTRequiresBearerAndAuditsDenial(t *testing.T) {
	handler, database := newProjectHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"unauthorized"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM authorization_denials WHERE reason='authentication_required'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("authentication denial audit count = %d, want 1", count)
	}
	largePath := httptest.NewRequest(http.MethodGet, "/api/v1/"+strings.Repeat("x", 4096), nil)
	largeResponse := httptest.NewRecorder()
	handler.ServeHTTP(largeResponse, largePath)
	var resourceLength int
	if err := database.QueryRow("SELECT length(resource) FROM authorization_denials ORDER BY sequence DESC LIMIT 1").Scan(&resourceLength); err != nil {
		t.Fatal(err)
	}
	if resourceLength > 256 {
		t.Fatalf("authorization audit resource length = %d, want at most 256", resourceLength)
	}
	if _, err := database.Exec("UPDATE authorization_denials SET reason='changed' WHERE sequence=1"); err == nil {
		t.Fatal("authorization denial audit was mutable")
	}
}

func TestLocalSessionExchangesBearerForHttpOnlyCookie(t *testing.T) {
	handler, _ := newProjectHandler(t)
	login := authorizedTestRequest(http.MethodPost, "/api/v1/session", nil)
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusNoContent {
		t.Fatalf("session status = %d, body = %s", loginResponse.Code, loginResponse.Body.String())
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != localAuthCookieName || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie = %+v, expected HttpOnly SameSite=Strict cookie", cookies)
	}
	projectRead := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	projectRead.AddCookie(cookies[0])
	projectResponse := httptest.NewRecorder()
	handler.ServeHTTP(projectResponse, projectRead)
	if projectResponse.Code != http.StatusOK {
		t.Fatalf("cookie-authenticated request status = %d, body = %s", projectResponse.Code, projectResponse.Body.String())
	}
	csrf := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"cross-site"}`))
	csrf.AddCookie(cookies[0])
	csrf.Header.Set("Origin", "http://attacker.example")
	csrfResponse := httptest.NewRecorder()
	handler.ServeHTTP(csrfResponse, csrf)
	if csrfResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-site cookie mutation status = %d, body = %s", csrfResponse.Code, csrfResponse.Body.String())
	}
	logout := httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logout)
	if logoutResponse.Code != http.StatusNoContent || logoutResponse.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatalf("logout response status = %d, cookies = %+v", logoutResponse.Code, logoutResponse.Result().Cookies())
	}
}

func TestDashboardCORSAllowsOnlyConfiguredOriginsAndHeaders(t *testing.T) {
	handler := corsAPI(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	allowed := httptest.NewRequest(http.MethodOptions, "/api/v1/session", nil)
	allowed.Header.Set("Origin", "http://localhost:4200")
	allowed.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	allowedResponse := httptest.NewRecorder()
	handler.ServeHTTP(allowedResponse, allowed)
	if allowedResponse.Code != http.StatusNoContent || allowedResponse.Header().Get("Access-Control-Allow-Origin") != "http://localhost:4200" || allowedResponse.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("allowed preflight = %d, headers = %v", allowedResponse.Code, allowedResponse.Header())
	}
	denied := httptest.NewRequest(http.MethodOptions, "/api/v1/session", nil)
	denied.Header.Set("Origin", "http://attacker.example")
	deniedResponse := httptest.NewRecorder()
	handler.ServeHTTP(deniedResponse, denied)
	if deniedResponse.Code != http.StatusForbidden || deniedResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("untrusted preflight = %d, headers = %v", deniedResponse.Code, deniedResponse.Header())
	}
}

func TestRESTEnforcesProjectMembership(t *testing.T) {
	handler, database := newProjectHandler(t)
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Scoped project"}`)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}
	var project projectResponse
	if err := json.Unmarshal(created.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("DELETE FROM project_members WHERE project_id=? AND user_id='local-administrator'", project.ID); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, "/api/v1/projects/"+project.ID, nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-member status = %d, body = %s", response.Code, response.Body.String())
	}
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM authorization_denials WHERE reason='project_membership_required'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("membership denial audit count = %d, want 1", count)
	}
}

func TestRESTAuditsDomainAuthorizationDenials(t *testing.T) {
	_, database := newProjectHandler(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeAPIError(w, http.StatusForbidden, "forbidden", "domain policy denied the operation")
	})
	handler := secureAPI(next, database, nil, nil, nil, testLocalAuthTokenBytes())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, "/api/v1/agents", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("domain denial status = %d, want %d", response.Code, http.StatusForbidden)
	}
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM authorization_denials WHERE reason='operation_denied' AND required_capability='agent.read'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("domain denial audit count = %d, want 1", count)
	}
}
