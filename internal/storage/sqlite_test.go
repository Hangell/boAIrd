package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenSQLiteAppliesMigrationsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "boaird.db")
	ctx := context.Background()

	database, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	database, err = OpenSQLite(ctx, path)
	if err != nil {
		t.Fatalf("reopen SQLite database: %v", err)
	}
	defer database.Close()

	var migrationCount int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("query migration count: %v", err)
	}
	if migrationCount != 1 {
		t.Fatalf("migration count = %d, want 1", migrationCount)
	}

	var tableName string
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'app_metadata'").Scan(&tableName); err != nil {
		t.Fatalf("verify bootstrap migration: %v", err)
	}
}
