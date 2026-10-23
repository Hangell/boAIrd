package template_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/project"
	"boAIrd/internal/storage"
	"boAIrd/internal/template"
)

func newTestService(t *testing.T) (*template.Service, *sql.DB) {
	t.Helper()
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
	for _, row := range []string{
		`INSERT INTO projects(id, name, status, human_review_required) VALUES ('p1', 'Active', 'CREATED', 1)`,
		`INSERT INTO projects(id, name, status, human_review_required) VALUES ('p2', 'Archived', 'ARCHIVED', 1)`,
	} {
		if _, err := database.Exec(row); err != nil {
			t.Fatal(err)
		}
	}
	projectRepository, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projectManager, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	templateRepository, err := storage.NewTemplateRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	service, err := template.NewService(templateRepository, projectManager)
	if err != nil {
		t.Fatal(err)
	}
	return service, database
}

func TestCreateGetListUpdate(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	value, err := service.Create(ctx, template.CreateInput{
		Key: "go-senior", Name: "Go Senior", Role: "backend", Instructions: "write idiomatic Go",
		DefaultRuntime: agent.RuntimeLocal, SuggestedTools: []string{"git", "shell"},
		SuggestedCapabilities: []string{"task.read", "task.review"},
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if value.Version != 1 {
		t.Fatalf("version = %d, want 1", value.Version)
	}
	got, err := service.Get(ctx, "go-senior")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Go Senior" || len(got.SuggestedTools) != 2 {
		t.Fatalf("template = %+v", got)
	}
	updated, err := service.Update(ctx, "go-senior", template.UpdateInput{
		Name: "Go Senior v2", Role: "backend", Instructions: "write idiomatic Go",
		DefaultRuntime: agent.RuntimeLocal, SuggestedTools: []string{"git"},
		SuggestedCapabilities: []string{"task.read"},
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.Name != "Go Senior v2" {
		t.Fatalf("updated = %+v", updated)
	}
	values, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 {
		t.Fatalf("list = %d templates, want 1", len(values))
	}
}

func TestCreateRejectsDuplicateAndInvalid(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	if _, err := service.Create(ctx, template.CreateInput{
		Key: "qa", Name: "QA", Role: "qa", Instructions: "review", DefaultRuntime: agent.RuntimeLocal,
	}, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, template.CreateInput{
		Key: "qa", Name: "QA", Role: "qa", Instructions: "review", DefaultRuntime: agent.RuntimeLocal,
	}, "admin"); err == nil {
		t.Fatal("expected duplicate template key to be rejected")
	}
	if _, err := service.Create(ctx, template.CreateInput{
		Key: "Bad Key", Name: "QA", Role: "qa", DefaultRuntime: agent.RuntimeLocal,
	}, "admin"); err == nil {
		t.Fatal("expected invalid key to be rejected")
	}
}

func TestInstantiateGrantsOnlyExplicitCapabilities(t *testing.T) {
	ctx := context.Background()
	service, database := newTestService(t)
	if _, err := service.Create(ctx, template.CreateInput{
		Key: "qa", Name: "QA", Role: "qa", Instructions: "review work",
		DefaultRuntime: agent.RuntimeLocal, SuggestedTools: []string{"filesystem"},
		SuggestedCapabilities: []string{"task.read", "task.review", "run.read"},
	}, "admin"); err != nil {
		t.Fatal(err)
	}
	created, err := service.Instantiate(ctx, "p1", "qa", template.InstantiateInput{
		Name: "QA instance", Capabilities: []string{"task.read"},
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Capabilities()) != 1 || created.Capabilities()[0] != "task.read" {
		t.Fatalf("instance capabilities = %v, want only [task.read]", created.Capabilities())
	}
	if !containsString(created.ProjectIDs(), "p1") {
		t.Fatalf("instance project scope = %v, want [p1]", created.ProjectIDs())
	}
	var version int
	if err := database.QueryRowContext(ctx, "SELECT template_version FROM template_instances WHERE agent_id = ?", created.ID()).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("instance template version = %d, want 1", version)
	}
	// The source template is unchanged by instantiation.
	source, err := service.Get(ctx, "qa")
	if err != nil {
		t.Fatal(err)
	}
	if source.Name != "QA" || len(source.SuggestedCapabilities) != 3 {
		t.Fatalf("source template mutated: %+v", source)
	}
}

func TestInstantiateAuditsChanges(t *testing.T) {
	ctx := context.Background()
	service, database := newTestService(t)
	if _, err := service.Create(ctx, template.CreateInput{
		Key: "qa", Name: "QA", Role: "qa", Instructions: "review", DefaultRuntime: agent.RuntimeLocal,
	}, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Instantiate(ctx, "p1", "qa", template.InstantiateInput{}, "admin"); err != nil {
		t.Fatal(err)
	}
	var created, updated, instantiated int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM template_audit WHERE template_key='qa' AND action='created'").Scan(&created); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM template_audit WHERE template_key='qa' AND action='updated'").Scan(&updated); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE project_id='p1' AND event_type='template.instantiated'").Scan(&instantiated); err != nil {
		t.Fatal(err)
	}
	if created != 1 || updated != 0 || instantiated != 1 {
		t.Fatalf("audit counts created=%d updated=%d instantiated=%d", created, updated, instantiated)
	}
}

func TestInstantiateRejectsUnknownOrArchivedProject(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	if _, err := service.Create(ctx, template.CreateInput{
		Key: "qa", Name: "QA", Role: "qa", Instructions: "review", DefaultRuntime: agent.RuntimeLocal,
	}, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Instantiate(ctx, "missing", "qa", template.InstantiateInput{}, "admin"); err == nil {
		t.Fatal("expected unknown project error")
	} else if !strings.Contains(err.Error(), template.ErrProjectNotFound.Error()) {
		t.Fatalf("error = %q, want ErrProjectNotFound", err)
	}
	if _, err := service.Instantiate(ctx, "p2", "qa", template.InstantiateInput{}, "admin"); err == nil {
		t.Fatal("expected archived project error")
	} else if !strings.Contains(err.Error(), template.ErrProjectArchived.Error()) {
		t.Fatalf("error = %q, want ErrProjectArchived", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestInstantiateAPITemplateRequiresProviderConfiguration(t *testing.T) {
	ctx := context.Background()
	service, database := newTestService(t)
	if _, err := service.Create(ctx, template.CreateInput{
		Key: "api-worker", Name: "API Worker", Role: "backend", Instructions: "call the provider",
		DefaultRuntime: agent.RuntimeAPI,
	}, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO credentials(id, name, provider, secret_ciphertext) VALUES ('cred-1','Key','openai', zeroblob(28))`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Instantiate(ctx, "p1", "api-worker", template.InstantiateInput{}, "admin"); err == nil {
		t.Fatal("expected API template instantiation without provider to fail")
	}
	created, err := service.Instantiate(ctx, "p1", "api-worker", template.InstantiateInput{
		Provider: "openai", Model: "gpt-x", CredentialID: "cred-1",
	}, "admin")
	if err != nil {
		t.Fatalf("instantiate API template with provider: %v", err)
	}
	if created.Runtime() != agent.RuntimeAPI {
		t.Fatalf("instantiated runtime = %q, want API", created.Runtime())
	}
}
