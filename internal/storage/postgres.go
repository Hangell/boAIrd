package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// OpenPostgres connects to PostgreSQL and applies its versioned migrations.
func OpenPostgres(ctx context.Context, connectionURL string) (*Database, error) {
	if connectionURL == "" {
		return nil, fmt.Errorf("PostgreSQL connection URL must not be empty")
	}
	db, err := sql.Open("pgx", connectionURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL database: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("connect to PostgreSQL database: %w", err), db.Close())
	}
	if err := migratePostgres(ctx, db); err != nil {
		return nil, errors.Join(fmt.Errorf("apply PostgreSQL migrations: %w", err), db.Close())
	}
	return &Database{connection: db, dialect: dialectPostgres}, nil
}

func migratePostgres(ctx context.Context, db *sql.DB) (result error) {
	connection, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire PostgreSQL migration connection: %w", err)
	}
	defer func() { result = errors.Join(result, connection.Close()) }()
	if _, err := connection.ExecContext(ctx, "SELECT pg_advisory_lock($1)", int64(0x626f41495244)); err != nil {
		return fmt.Errorf("lock PostgreSQL migrations: %w", err)
	}
	defer func() {
		_, _ = connection.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", int64(0x626f41495244))
	}()
	if _, err := connection.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT (to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'))
	)`); err != nil {
		return fmt.Errorf("create PostgreSQL schema_migrations table: %w", err)
	}
	files, err := fs.Glob(migrationFiles, "migrations/postgres/*.sql")
	if err != nil {
		return fmt.Errorf("list PostgreSQL migrations: %w", err)
	}
	migrations := make([]migration, 0, len(files))
	for _, name := range files {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		migrations = append(migrations, migration{name: name, version: version})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	for index, value := range migrations {
		if index > 0 && migrations[index-1].version == value.version {
			return fmt.Errorf("duplicate PostgreSQL migration version %d", value.version)
		}
		var applied bool
		if err := connection.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", value.version).Scan(&applied); err != nil {
			return fmt.Errorf("check PostgreSQL migration %s: %w", value.name, err)
		}
		if applied {
			continue
		}
		script, err := migrationFiles.ReadFile(value.name)
		if err != nil {
			return fmt.Errorf("read PostgreSQL migration %s: %w", value.name, err)
		}
		tx, err := connection.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin PostgreSQL migration %s: %w", value.name, err)
		}
		for _, statement := range splitPostgresStatements(string(script)) {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return errors.Join(fmt.Errorf("execute PostgreSQL migration %s: %w", value.name, err), tx.Rollback())
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, name) VALUES ($1, $2)", value.version, value.name); err != nil {
			return errors.Join(fmt.Errorf("record PostgreSQL migration %s: %w", value.name, err), tx.Rollback())
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit PostgreSQL migration %s: %w", value.name, err)
		}
	}
	return nil
}

func splitPostgresStatements(script string) []string {
	statements := make([]string, 0)
	start := 0
	state := byte(0)
	dollarTag := ""
	for index := 0; index < len(script); index++ {
		current := script[index]
		if dollarTag != "" {
			if strings.HasPrefix(script[index:], dollarTag) {
				index += len(dollarTag) - 1
				dollarTag = ""
			}
			continue
		}
		switch state {
		case '\'':
			if current == '\'' {
				if index+1 < len(script) && script[index+1] == '\'' {
					index++
				} else {
					state = 0
				}
			}
		case '"':
			if current == '"' {
				state = 0
			}
		case 'l':
			if current == '\n' {
				state = 0
			}
		case 'b':
			if current == '*' && index+1 < len(script) && script[index+1] == '/' {
				state = 0
				index++
			}
		default:
			switch {
			case current == '\'':
				state = '\''
			case current == '"':
				state = '"'
			case current == '-' && index+1 < len(script) && script[index+1] == '-':
				state = 'l'
				index++
			case current == '/' && index+1 < len(script) && script[index+1] == '*':
				state = 'b'
				index++
			case current == '$':
				if end := strings.IndexByte(script[index+1:], '$'); end >= 0 {
					tag := script[index+1 : index+1+end]
					if isDollarTag(tag) {
						dollarTag = script[index : index+end+2]
						index += len(dollarTag) - 1
					}
				}
			case current == ';':
				if statement := strings.TrimSpace(script[start:index]); statement != "" {
					statements = append(statements, statement)
				}
				start = index + 1
			}
		}
	}
	if statement := strings.TrimSpace(script[start:]); statement != "" {
		statements = append(statements, statement)
	}
	return statements
}

func isDollarTag(tag string) bool {
	for index, value := range tag {
		if value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' {
			continue
		}
		if index > 0 && value >= '0' && value <= '9' {
			continue
		}
		return false
	}
	return true
}
