package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// OpenSQLite creates or opens the SQLite database and applies pending migrations.
func OpenSQLite(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("database path must not be empty")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	dbFile, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create database file: %w", err)
	}
	if err := dbFile.Chmod(0o600); err != nil {
		return nil, errors.Join(fmt.Errorf("restrict database file permissions: %w", err), dbFile.Close())
	}
	if err := dbFile.Close(); err != nil {
		return nil, fmt.Errorf("close database file: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("connect to SQLite database: %w", err), db.Close())
	}
	if err := migrate(ctx, db); err != nil {
		return nil, errors.Join(fmt.Errorf("apply SQLite migrations: %w", err), db.Close())
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	files, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list migration files: %w", err)
	}
	migrations := make([]migration, 0, len(files))
	for _, name := range files {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		migrations = append(migrations, migration{name: name, version: version})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	for i, migration := range migrations {
		if i > 0 && migrations[i-1].version == migration.version {
			return fmt.Errorf("duplicate migration version %d", migration.version)
		}

		var applied bool
		if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", migration.version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", migration.name, err)
		}
		if applied {
			continue
		}

		script, err := migrationFiles.ReadFile(migration.name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", migration.name, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", migration.name, err)
		}
		if _, err := tx.ExecContext(ctx, string(script)); err != nil {
			return errors.Join(fmt.Errorf("execute migration %s: %w", migration.name, err), tx.Rollback())
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, name) VALUES (?, ?)", migration.version, filepath.Base(migration.name)); err != nil {
			return errors.Join(fmt.Errorf("record migration %s: %w", migration.name, err), tx.Rollback())
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", migration.name, err)
		}
	}
	return nil
}

type migration struct {
	name    string
	version int
}

func migrationVersion(name string) (int, error) {
	base := filepath.Base(name)
	separator := strings.IndexByte(base, '_')
	if separator < 1 {
		return 0, fmt.Errorf("invalid migration filename %q: expected <version>_<name>.sql", base)
	}
	version, err := strconv.Atoi(base[:separator])
	if err != nil || version < 1 {
		return 0, fmt.Errorf("invalid migration version in filename %q", base)
	}
	return version, nil
}
