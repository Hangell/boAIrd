package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/project"
	"boAIrd/internal/storage"
)

func TestProjectLifecycleEndpoints(t *testing.T) {
	handler, _ := newProjectHandler(t)

	create := authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":" Alpha "}`))
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}
	var projectBody projectResponse
	if err := json.Unmarshal(created.Body.Bytes(), &projectBody); err != nil {
		t.Fatal(err)
	}
	if projectBody.Name != "Alpha" || projectBody.Status != project.StatusCreated || !projectBody.HumanReviewRequired {
		t.Fatalf("unexpected project defaults: %+v", projectBody)
	}
	if created.Header().Get("Location") != "/api/v1/projects/"+projectBody.ID {
		t.Fatalf("unexpected Location header %q", created.Header().Get("Location"))
	}

	list := request(t, handler, http.MethodGet, "/api/v1/projects?limit=10&offset=0")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"name":"Alpha"`) {
		t.Fatalf("list status = %d, body = %s", list.Code, list.Body.String())
	}

	patch := authorizedTestRequest(http.MethodPatch, "/api/v1/projects/"+projectBody.ID, strings.NewReader(`{"description":"managed","human_review_required":false}`))
	updated := httptest.NewRecorder()
	handler.ServeHTTP(updated, patch)
	if updated.Code != http.StatusOK || strings.Contains(updated.Body.String(), `"autonomy_mode"`) || !strings.Contains(updated.Body.String(), `"human_review_required":false`) {
		t.Fatalf("update status = %d, body = %s", updated.Code, updated.Body.String())
	}

	delete := request(t, handler, http.MethodDelete, "/api/v1/projects/"+projectBody.ID)
	if delete.Code != http.StatusOK || !strings.Contains(delete.Body.String(), `"status":"ARCHIVED"`) {
		t.Fatalf("archive status = %d, body = %s", delete.Code, delete.Body.String())
	}
	deleteAgain := request(t, handler, http.MethodDelete, "/api/v1/projects/"+projectBody.ID)
	if deleteAgain.Code != http.StatusOK {
		t.Fatalf("repeated archive status = %d, body = %s", deleteAgain.Code, deleteAgain.Body.String())
	}

	get := request(t, handler, http.MethodGet, "/api/v1/projects/"+projectBody.ID)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"status":"ARCHIVED"`) {
		t.Fatalf("get status = %d, body = %s", get.Code, get.Body.String())
	}
}

func TestProjectEndpointsValidateWorkspaceAndUnknownFields(t *testing.T) {
	handler, _ := newProjectHandler(t)
	workspace := t.TempDir()
	body := `{"name":"Workspace project","workspace_path":"` + workspace + `"}`
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body)))
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), workspace) {
		t.Fatalf("valid workspace create status = %d, body = %s", created.Code, created.Body.String())
	}

	invalidPath := httptest.NewRecorder()
	traversal := workspace + string(filepath.Separator) + ".." + string(filepath.Separator) + "escape"
	badPathJSON, err := json.Marshal(map[string]string{"name": "Invalid", "workspace_path": traversal})
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(invalidPath, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(string(badPathJSON))))
	if invalidPath.Code != http.StatusBadRequest {
		t.Fatalf("invalid workspace status = %d, body = %s", invalidPath.Code, invalidPath.Body.String())
	}

	unknownField := httptest.NewRecorder()
	handler.ServeHTTP(unknownField, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Unknown","status":"RUNNING"}`)))
	if unknownField.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, body = %s", unknownField.Code, unknownField.Body.String())
	}

	invalidLimit := request(t, handler, http.MethodGet, "/api/v1/projects?limit=0")
	if invalidLimit.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, body = %s", invalidLimit.Code, invalidLimit.Body.String())
	}
}

func TestProjectEndpointsReturnNotFoundAndRejectInvalidSettings(t *testing.T) {
	handler, _ := newProjectHandler(t)
	missing := request(t, handler, http.MethodGet, "/api/v1/projects/missing")
	if missing.Code != http.StatusForbidden {
		t.Fatalf("unassociated project status = %d, body = %s", missing.Code, missing.Body.String())
	}

	removedAutonomy := httptest.NewRecorder()
	handler.ServeHTTP(removedAutonomy, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Invalid","autonomy_mode":"MANUAL"}`)))
	if removedAutonomy.Code != http.StatusBadRequest {
		t.Fatalf("project autonomy field should be rejected: status = %d, body = %s", removedAutonomy.Code, removedAutonomy.Body.String())
	}
}

func TestProjectBrainAssociationRequiresAnActiveBrain(t *testing.T) {
	handler, database := newProjectHandler(t)
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Brain project"}`)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}
	var value projectResponse
	if err := json.Unmarshal(created.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}

	patch := func(brainID string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]string{"brain_id": brainID})
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, authorizedTestRequest(http.MethodPatch, "/api/v1/projects/"+value.ID, strings.NewReader(string(body))))
		return recorder
	}
	if response := patch("brain-1"); response.Code != http.StatusBadRequest {
		t.Fatalf("inactive Brain status = %d, body = %s", response.Code, response.Body.String())
	}
	secondProject := httptest.NewRecorder()
	handler.ServeHTTP(secondProject, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Other project"}`)))
	var other projectResponse
	if err := json.Unmarshal(secondProject.Body.Bytes(), &other); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO brains(id, project_id, active) VALUES ('brain-2', ?, 1)`, other.ID); err != nil {
		t.Fatal(err)
	}
	if response := patch("brain-2"); response.Code != http.StatusBadRequest {
		t.Fatalf("Brain belonging to another project status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := database.Exec(`INSERT INTO brains(id, project_id, active) VALUES ('brain-1', ?, 1)`, value.ID); err != nil {
		t.Fatal(err)
	}
	if response := patch("brain-1"); response.Code != http.StatusOK {
		t.Fatalf("active Brain status = %d, body = %s", response.Code, response.Body.String())
	}
}

func newProjectHandler(t *testing.T) (http.Handler, *storage.Database) {
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
	repository, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := project.NewManager(repository)
	if err != nil {
		t.Fatal(err)
	}
	return newHandler(database, manager, nil, nil, nil, nil, nil, nil, testLocalAuthTokenBytes()), database
}
