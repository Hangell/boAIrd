package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"boAIrd/internal/capability"
	"boAIrd/internal/extension"
)

func TestExtensionRepositoryPersistsLifecycleAndReservesNames(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "extensions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	repository, err := NewExtensionRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	value := testExtension(t, "example-tools", "example.search")
	if err := repository.InstallWithAudit(ctx, value, "admin"); err != nil {
		t.Fatal(err)
	}
	if active, err := repository.IsActiveTool(ctx, "example.search"); err != nil || active {
		t.Fatalf("pending tool active = %v, error = %v; want false", active, err)
	}
	if err := repository.InstallWithAudit(ctx, value, "admin"); !errors.Is(err, extension.ErrAlreadyExists) {
		t.Fatalf("duplicate install error = %v, want already exists", err)
	}
	if err := repository.InstallWithAudit(ctx, testExtension(t, "other-tools", "example.search"), "admin"); !errors.Is(err, extension.ErrToolNameConflict) {
		t.Fatalf("duplicate tool install error = %v, want tool-name conflict", err)
	}
	got, err := repository.Get(ctx, "example-tools")
	if err != nil || len(got.Tools()) != 1 || got.Tools()[0].Name != "example.search" {
		t.Fatalf("Get() = %#v, error = %v", got, err)
	}
	if err := repository.ActivateWithAudit(ctx, "example-tools", "admin"); err != nil {
		t.Fatal(err)
	}
	if active, err := repository.IsActiveTool(ctx, "example.search"); err != nil || !active {
		t.Fatalf("activated tool active = %v, error = %v; want true", active, err)
	}
	values, err := repository.List(ctx)
	if err != nil || len(values) != 1 || values[0].Status() != extension.StatusActive {
		t.Fatalf("List() = %#v, error = %v", values, err)
	}
	if err := repository.DeactivateWithAudit(ctx, "example-tools", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Get(ctx, "example-tools"); !errors.Is(err, extension.ErrNotFound) {
		t.Fatalf("Get() after deactivation error = %v, want not found", err)
	}
	if active, err := repository.IsActiveTool(ctx, "example.search"); err != nil || active {
		t.Fatalf("deactivated tool active = %v, error = %v; want false", active, err)
	}
	if err := repository.InstallWithAudit(ctx, testExtension(t, "replacement-tools", "example.search"), "admin"); err != nil {
		t.Fatalf("install after deactivation: %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE extension_audit SET actor_id = 'rewritten'`); err == nil {
		t.Fatal("extension audit update unexpectedly succeeded")
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM extension_audit`); err == nil {
		t.Fatal("extension audit delete unexpectedly succeeded")
	}
}

func TestExtensionLifecycleAuditFailuresRollBackState(t *testing.T) {
	ctx := context.Background()
	database, err := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "extensions-atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	repository, err := NewExtensionRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	value := testExtension(t, "atomic-tools", "atomic.search")
	if err := repository.InstallWithAudit(ctx, value, "admin"); err != nil {
		t.Fatal(err)
	}
	createAuditFailureTrigger(t, database, "extension.activated")
	if err := repository.ActivateWithAudit(ctx, value.Name(), "admin"); err == nil {
		t.Fatal("activation with failed audit unexpectedly succeeded")
	}
	dropAuditFailureTrigger(t, database)
	stored, err := repository.Get(ctx, value.Name())
	if err != nil || stored.Status() != extension.StatusPending {
		t.Fatalf("status after failed activation = %s, error = %v; want pending", stored.Status(), err)
	}
	if err := repository.ActivateWithAudit(ctx, value.Name(), "admin"); err != nil {
		t.Fatal(err)
	}
	createAuditFailureTrigger(t, database, "extension.deactivated")
	if err := repository.DeactivateWithAudit(ctx, value.Name(), "admin"); err == nil {
		t.Fatal("deactivation with failed audit unexpectedly succeeded")
	}
	dropAuditFailureTrigger(t, database)
	stored, err = repository.Get(ctx, value.Name())
	if err != nil || stored.Status() != extension.StatusActive {
		t.Fatalf("status after failed deactivation = %s, error = %v; want active", stored.Status(), err)
	}
	if active, err := repository.IsActiveTool(ctx, "atomic.search"); err != nil || !active {
		t.Fatalf("tool after failed deactivation active = %v, error = %v; want true", active, err)
	}
	if err := repository.InstallWithAudit(ctx, testExtension(t, "reserved-tools", "atomic.search"), "admin"); !errors.Is(err, extension.ErrToolNameConflict) {
		t.Fatalf("name reservation after failed deactivation error = %v, want tool-name conflict", err)
	}
}

func testExtension(t *testing.T, name, toolName string) extension.Extension {
	t.Helper()
	value, err := extension.Restore(name, "1.0.0", extension.StatusPending, []extension.ToolDeclaration{{
		Name: toolName, Description: "Search", Kind: extension.KindDeclarative,
		InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`),
		RequiredCapabilities: []capability.Capability{"extension.search"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func createAuditFailureTrigger(t *testing.T, database *Database, eventType string) {
	t.Helper()
	ctx := context.Background()
	if database.dialect == dialectSQLite {
		_, err := database.ExecContext(ctx, `CREATE TRIGGER test_fail_extension_audit BEFORE INSERT ON extension_audit
			WHEN NEW.event_type = '`+eventType+`' BEGIN SELECT RAISE(ABORT, 'test audit failure'); END`)
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	_, err := database.ExecContext(ctx, `CREATE FUNCTION test_fail_extension_audit_fn() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.event_type = '`+eventType+`' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END $$`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `CREATE TRIGGER test_fail_extension_audit BEFORE INSERT ON extension_audit
		FOR EACH ROW EXECUTE FUNCTION test_fail_extension_audit_fn()`); err != nil {
		t.Fatal(err)
	}
}

func dropAuditFailureTrigger(t *testing.T, database *Database) {
	t.Helper()
	ctx := context.Background()
	if database.dialect == dialectSQLite {
		if _, err := database.ExecContext(ctx, `DROP TRIGGER test_fail_extension_audit`); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := database.ExecContext(ctx, `DROP TRIGGER test_fail_extension_audit ON extension_audit`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `DROP FUNCTION test_fail_extension_audit_fn()`); err != nil {
		t.Fatal(err)
	}
}
