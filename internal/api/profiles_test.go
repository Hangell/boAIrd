package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/profile"
	"boAIrd/internal/project"
	"boAIrd/internal/storage"
)

func newProfileHandler(t *testing.T) (http.Handler, *profile.Service, string) {
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
	profileRepository, err := storage.NewProfileRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	service, err := profile.NewService(profileRepository, projectManager)
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandlerWithOrchestration(fakeDatabase{}, projectManager, nil, nil, nil, nil, nil, nil, testLocalAuthTokenBytes(), nil, nil, service, nil)

	created := httptest.NewRecorder()
	handler.ServeHTTP(created, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Profile target"}`)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create project status = %d, body = %s", created.Code, created.Body.String())
	}
	var value projectResponse
	if err := json.Unmarshal(created.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return handler, service, value.ID
}

func TestProfileCatalogListsBuiltins(t *testing.T) {
	handler, _, _ := newProfileHandler(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, "/api/v1/profiles", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var page profileListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != len(profile.Builtins()) {
		t.Fatalf("profiles = %d, want %d", len(page.Items), len(profile.Builtins()))
	}
}

func TestProjectProfilesEffectiveAndCustomization(t *testing.T) {
	handler, _, projectID := newProfileHandler(t)

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, authorizedTestRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/profiles", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", list.Code, list.Body.String())
	}
	var page effectiveProfileListResponse
	if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if len(item.Capabilities) != 0 {
			t.Fatalf("uncustomized profile %q granted capabilities %v", item.Key, item.Capabilities)
		}
	}

	put := httptest.NewRecorder()
	handler.ServeHTTP(put, authorizedTestRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/profiles/qa",
		strings.NewReader(`{"instructions":"custom qa","capabilities":["task.read","task.review"],"tools":["filesystem"]}`)))
	if put.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", put.Code, put.Body.String())
	}

	get := httptest.NewRecorder()
	handler.ServeHTTP(get, authorizedTestRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/profiles/qa", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", get.Code, get.Body.String())
	}
	var value effectiveProfileResponse
	if err := json.Unmarshal(get.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if !value.Customized || value.Instructions != "custom qa" || len(value.Capabilities) != 2 || len(value.Tools) != 1 {
		t.Fatalf("effective profile = %+v", value)
	}
}

func TestProfileRejectsInvalidAndUnauthenticated(t *testing.T) {
	handler, _, projectID := newProfileHandler(t)

	badTool := httptest.NewRecorder()
	handler.ServeHTTP(badTool, authorizedTestRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/profiles/qa",
		strings.NewReader(`{"tools":["shell"]}`)))
	if badTool.Code != http.StatusBadRequest {
		t.Fatalf("bad tool status = %d, want 400", badTool.Code)
	}

	unknown := httptest.NewRecorder()
	handler.ServeHTTP(unknown, authorizedTestRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/profiles/unknown", strings.NewReader(`{}`)))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown profile status = %d, want 404", unknown.Code)
	}

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/profiles/qa", strings.NewReader(`{}`)))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", unauthenticated.Code)
	}
}
