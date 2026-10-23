package storage

import (
	"context"
	"os"
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
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	var migrationCount int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("query migration count: %v", err)
	}
	if migrationCount != 25 {
		t.Fatalf("migration count = %d, want 25", migrationCount)
	}

	var tableName string
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'app_metadata'").Scan(&tableName); err != nil {
		t.Fatalf("verify bootstrap migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'projects'").Scan(&tableName); err != nil {
		t.Fatalf("verify projects migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'brains'").Scan(&tableName); err != nil {
		t.Fatalf("verify active Brain registry migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'credentials'").Scan(&tableName); err != nil {
		t.Fatalf("verify credential vault migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'agents'").Scan(&tableName); err != nil {
		t.Fatalf("verify agent configuration migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'runs'").Scan(&tableName); err != nil {
		t.Fatalf("verify Brain run migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'audit_events'").Scan(&tableName); err != nil {
		t.Fatalf("verify immutable audit events migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'tasks'").Scan(&tableName); err != nil {
		t.Fatalf("verify Kanban task migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'conversations'").Scan(&tableName); err != nil {
		t.Fatalf("verify task conversation migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'conversation_messages'").Scan(&tableName); err != nil {
		t.Fatalf("verify conversation message migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'project_members'").Scan(&tableName); err != nil {
		t.Fatalf("verify project membership migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'agent_projects'").Scan(&tableName); err != nil {
		t.Fatalf("verify Agent project scope migration: %v", err)
	}
	var versionColumn string
	if err := database.QueryRowContext(ctx, "SELECT name FROM pragma_table_info('tasks') WHERE name='version'").Scan(&versionColumn); err != nil {
		t.Fatalf("verify task version migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'project_memory'").Scan(&tableName); err != nil {
		t.Fatalf("verify project memory migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'project_profiles'").Scan(&tableName); err != nil {
		t.Fatalf("verify project profiles migration: %v", err)
	}
	var resultColumn string
	if err := database.QueryRowContext(ctx, "SELECT name FROM pragma_table_info('audit_events') WHERE name='result'").Scan(&resultColumn); err != nil {
		t.Fatalf("verify audit result migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'notifications'").Scan(&tableName); err != nil {
		t.Fatalf("verify notifications migration: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'agent_templates'").Scan(&tableName); err != nil {
		t.Fatalf("verify agent templates migration: %v", err)
	}
}

func TestOpenSQLiteRestrictsExistingDatabasePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boaird.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("create permissive database file: %v", err)
	}

	database, err := OpenSQLite(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat database file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("database permissions = %#o, want %#o", got, 0o600)
	}
}
