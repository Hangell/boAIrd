package profile_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/profile"
	"boAIrd/internal/storage"
)

func newTestService(t *testing.T) (*profile.Service, *sql.DB) {
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
		`INSERT INTO projects(id, name, status, human_review_required) VALUES ('p2', 'Other', 'CREATED', 1)`,
		`INSERT INTO projects(id, name, status, human_review_required) VALUES ('p3', 'Archived', 'ARCHIVED', 1)`,
	} {
		if _, err := database.Exec(row); err != nil {
			t.Fatal(err)
		}
	}
	projects, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := storage.NewProfileRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := storage.NewAuditRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	service, err := profile.NewService(repository, projects, audit)
	if err != nil {
		t.Fatal(err)
	}
	return service, database
}

func TestEffectiveGrantsNothingWithoutOverride(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	effective, err := service.Effective(ctx, "p1", "qa")
	if err != nil {
		t.Fatal(err)
	}
	if len(effective.Capabilities) != 0 || len(effective.Tools) != 0 {
		t.Fatalf("profile name implicitly granted capabilities=%v tools=%v", effective.Capabilities, effective.Tools)
	}
	if effective.Customized {
		t.Fatal("expected uncustomized profile")
	}
	if effective.Instructions == "" || effective.Name == "" {
		t.Fatal("expected built-in instructions and name")
	}
}

func TestSaveAppliesExplicitConfiguration(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	effective, err := service.Save(ctx, "p1", "qa", profile.SaveInput{
		Instructions: "custom instructions", Capabilities: []string{"task.read", "task.review"},
		Tools: []string{"filesystem"}, ActorID: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !effective.Customized || effective.Instructions != "custom instructions" {
		t.Fatalf("effective = %+v", effective)
	}
	if len(effective.Capabilities) != 2 || effective.Capabilities[0] != "task.read" || effective.Capabilities[1] != "task.review" {
		t.Fatalf("capabilities = %v", effective.Capabilities)
	}
	if len(effective.Tools) != 1 || effective.Tools[0] != "filesystem" {
		t.Fatalf("tools = %v", effective.Tools)
	}
	// Suggested capabilities remain available as reference but are not granted.
	if len(effective.SuggestedCapabilities) < 2 {
		t.Fatalf("suggested capabilities missing: %v", effective.SuggestedCapabilities)
	}
}

func TestSaveRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	tests := []struct {
		name    string
		project string
		key     string
		input   profile.SaveInput
	}{
		{name: "unknown profile", project: "p1", key: "unknown", input: profile.SaveInput{ActorID: "admin"}},
		{name: "unavailable tool", project: "p1", key: "qa", input: profile.SaveInput{Tools: []string{"shell"}, ActorID: "admin"}},
		{name: "invalid capability", project: "p1", key: "qa", input: profile.SaveInput{Capabilities: []string{"Not-A-Cap"}, ActorID: "admin"}},
		{name: "missing actor", project: "p1", key: "qa", input: profile.SaveInput{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.Save(ctx, tt.project, tt.key, tt.input); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSaveAuditsCreatedAndUpdated(t *testing.T) {
	ctx := context.Background()
	service, database := newTestService(t)
	if _, err := service.Save(ctx, "p1", "qa", profile.SaveInput{ActorID: "admin", Capabilities: []string{"task.read"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(ctx, "p1", "qa", profile.SaveInput{ActorID: "admin", Capabilities: []string{"task.read", "task.review"}}); err != nil {
		t.Fatal(err)
	}
	var created, updated int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE project_id='p1' AND resource_type='profile' AND resource_id='qa' AND event_type='profile.created'").Scan(&created); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE project_id='p1' AND resource_type='profile' AND resource_id='qa' AND event_type='profile.updated'").Scan(&updated); err != nil {
		t.Fatal(err)
	}
	if created != 1 || updated != 1 {
		t.Fatalf("audit events created=%d updated=%d, want 1 and 1", created, updated)
	}
}

func TestProjectIsolation(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	if _, err := service.Save(ctx, "p1", "qa", profile.SaveInput{ActorID: "admin", Instructions: "p1 only", Capabilities: []string{"task.read"}}); err != nil {
		t.Fatal(err)
	}
	other, err := service.Effective(ctx, "p2", "qa")
	if err != nil {
		t.Fatal(err)
	}
	if other.Customized || other.Instructions == "p1 only" || len(other.Capabilities) != 0 {
		t.Fatalf("project isolation violated: %+v", other)
	}
}

func TestUnknownOrArchivedProjectRejected(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	if _, err := service.Effective(ctx, "missing", "qa"); err == nil {
		t.Fatal("expected unknown project error")
	} else if !strings.Contains(err.Error(), profile.ErrProjectNotFound.Error()) {
		t.Fatalf("error = %q, want ErrProjectNotFound", err)
	}
	if _, err := service.Effective(ctx, "p3", "qa"); err == nil {
		t.Fatal("expected archived project error")
	} else if !strings.Contains(err.Error(), profile.ErrProjectArchived.Error()) {
		t.Fatalf("error = %q, want ErrProjectArchived", err)
	}
}

func TestListEffectiveIncludesAllBuiltins(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	if _, err := service.Save(ctx, "p1", "qa", profile.SaveInput{ActorID: "admin", Capabilities: []string{"task.read"}}); err != nil {
		t.Fatal(err)
	}
	values, err := service.ListEffective(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != len(profile.Builtins()) {
		t.Fatalf("effective profiles = %d, want %d", len(values), len(profile.Builtins()))
	}
	for _, value := range values {
		if value.Key == "qa" && (!value.Customized || len(value.Capabilities) != 1) {
			t.Fatalf("qa profile not customized as expected: %+v", value)
		}
	}
}
