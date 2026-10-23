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

func TestExtensionRepositoryPersistsLifecycle(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "extensions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	repository, err := NewExtensionRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	value, err := extension.Restore("example-tools", "1.0.0", extension.StatusPending, []extension.ToolDeclaration{{
		Name: "example.search", Description: "Search", Kind: extension.KindDeclarative,
		InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`),
		RequiredCapabilities: []capability.Capability{"example.search"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, value); err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, value); !errors.Is(err, extension.ErrAlreadyExists) {
		t.Fatalf("duplicate Create() error = %v, want already exists", err)
	}
	got, err := repository.Get(ctx, "example-tools")
	if err != nil || len(got.Tools()) != 1 || got.Tools()[0].Name != "example.search" {
		t.Fatalf("Get() = %#v, error = %v", got, err)
	}
	if err := repository.SetStatus(ctx, "example-tools", extension.StatusActive); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordExtensionEvent(ctx, "admin", "example-tools", "extension.activated"); err != nil {
		t.Fatal(err)
	}
	values, err := repository.List(ctx)
	if err != nil || len(values) != 1 || values[0].Status() != extension.StatusActive {
		t.Fatalf("List() = %#v, error = %v", values, err)
	}
	if err := repository.Delete(ctx, "example-tools"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Get(ctx, "example-tools"); !errors.Is(err, extension.ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want not found", err)
	}
}
