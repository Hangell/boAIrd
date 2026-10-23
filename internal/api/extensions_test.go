package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/extension"
	"boAIrd/internal/storage"
)

type registrarStub struct{}

func (registrarStub) RegisterTools(_ context.Context, _ []extension.ToolDeclaration) error {
	return nil
}
func (registrarStub) UnregisterTools(_ context.Context, _ []string) {}

func newExtensionHandler(t *testing.T) (http.Handler, *storage.Database) {
	t.Helper()
	database, err := storage.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "extensions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	repository, err := storage.NewExtensionRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	service, err := extension.NewService(repository, registrarStub{})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandlerWithSecurityNotificationsTemplatesExtensionsAndUsers(
		database, nil, nil, nil, nil, nil, nil, nil, testLocalAuthTokenBytes(), nil, nil,
		nil, nil, nil, nil, service, bootstrapTestUserManager{}, SecurityPolicy{},
	)
	return handler, database
}

func TestExtensionAPILifecycle(t *testing.T) {
	handler, _ := newExtensionHandler(t)
	manifest := `{"sdk_version":1,"name":"example-tools","version":"1.0.0","tools":[{"name":"example.search","description":"Search","kind":"declarative","input_schema":{"type":"object"},"output_schema":{"type":"object"},"required_capabilities":["example.search"]}]}`
	install := httptest.NewRecorder()
	handler.ServeHTTP(install, authorizedTestRequest(http.MethodPost, "/api/v1/extensions", strings.NewReader(manifest)))
	if install.Code != http.StatusCreated {
		t.Fatalf("install extension status = %d, body = %s", install.Code, install.Body.String())
	}
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, authorizedTestRequest(http.MethodGet, "/api/v1/extensions", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"example-tools"`) {
		t.Fatalf("list extensions status = %d, body = %s", list.Code, list.Body.String())
	}
	activate := httptest.NewRecorder()
	handler.ServeHTTP(activate, authorizedTestRequest(http.MethodPut, "/api/v1/extensions/example-tools/activate", nil))
	if activate.Code != http.StatusOK || !strings.Contains(activate.Body.String(), `"active"`) {
		t.Fatalf("activate extension status = %d, body = %s", activate.Code, activate.Body.String())
	}
	deactivate := httptest.NewRecorder()
	handler.ServeHTTP(deactivate, authorizedTestRequest(http.MethodDelete, "/api/v1/extensions/example-tools", nil))
	if deactivate.Code != http.StatusNoContent {
		t.Fatalf("deactivate extension status = %d, body = %s", deactivate.Code, deactivate.Body.String())
	}
}

func TestExtensionAPIRejectsInvalidManifest(t *testing.T) {
	handler, _ := newExtensionHandler(t)
	invalid := `{"sdk_version":2,"name":"bad","version":"1","tools":[{"name":"t","description":"x","kind":"shell","input_schema":{"type":"object"},"output_schema":{"type":"object"},"required_capabilities":["x.y"]}]}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodPost, "/api/v1/extensions", strings.NewReader(invalid)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("install invalid manifest status = %d, body = %s; want 400", response.Code, response.Body.String())
	}
}

func TestExtensionAPIRequiresAuthentication(t *testing.T) {
	handler, _ := newExtensionHandler(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/extensions", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated extensions status = %d, want 401", response.Code)
	}
}
