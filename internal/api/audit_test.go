package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/audit"
	"boAIrd/internal/project"
	"boAIrd/internal/storage"
)

func TestAuditAPIReadsProjectScopedRedactedEvents(t *testing.T) {
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
	auditRepository, err := storage.NewAuditRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandlerWithOrchestration(fakeDatabase{}, projectManager, nil, nil, nil, nil, nil, nil, testLocalAuthTokenBytes(), nil, nil, auditRepository)

	created := httptest.NewRecorder()
	handler.ServeHTTP(created, authorizedTestRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Audit target"}`)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create project status = %d, body = %s", created.Code, created.Body.String())
	}
	var value projectResponse
	if err := json.Unmarshal(created.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}

	if err := auditRepository.RecordEvent(context.Background(), audit.Event{
		ProjectID: value.ID, ActorType: "administrator", ActorID: "local-administrator",
		Action: "agent.capability.granted", ResourceType: "agent", ResourceID: "agent-1",
		Result: "success", Reason: "granted shell.execute token=must-not-persist",
	}); err != nil {
		t.Fatal(err)
	}

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, authorizedTestRequest(http.MethodGet, "/api/v1/projects/"+value.ID+"/audit", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("audit list status = %d, body = %s", list.Code, list.Body.String())
	}
	var page auditListResponse
	if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Action != "agent.capability.granted" {
		t.Fatalf("audit items = %+v", page.Items)
	}
	if strings.Contains(page.Items[0].Reason, "must-not-persist") {
		t.Fatalf("audit response leaked secret: %q", page.Items[0].Reason)
	}

	outOfScope := httptest.NewRecorder()
	handler.ServeHTTP(outOfScope, authorizedTestRequest(http.MethodGet, "/api/v1/projects/missing-project/audit", nil))
	if outOfScope.Code != http.StatusForbidden {
		t.Fatalf("out-of-scope audit status = %d, want 403", outOfScope.Code)
	}

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+value.ID+"/audit", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated audit status = %d, want 401", unauthenticated.Code)
	}
}
