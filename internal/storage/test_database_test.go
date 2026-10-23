package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
)

func openTestDatabase(t *testing.T, ctx context.Context, sqlitePath string) (*Database, error) {
	t.Helper()
	connectionURL := os.Getenv("BOAIRD_TEST_POSTGRES_URL")
	if connectionURL == "" {
		database, err := OpenSQLite(ctx, sqlitePath)
		if err != nil {
			return nil, err
		}
		return database, nil
	}

	parsed, err := url.Parse(connectionURL)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL test URL: %w", err)
	}
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return nil, fmt.Errorf("generate PostgreSQL test schema: %w", err)
	}
	schema := "boaird_test_" + hex.EncodeToString(suffix[:])
	admin, err := sql.Open("pgx", connectionURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL schema connection: %w", err)
	}
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		_ = admin.Close()
		return nil, fmt.Errorf("create PostgreSQL test schema: %w", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	database, err := OpenPostgres(ctx, parsed.String())
	if err != nil {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`)
		_ = admin.Close()
		return nil, fmt.Errorf("open PostgreSQL test database: %w", err)
	}
	database.cleanup = func() error {
		_, dropErr := admin.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA "%s" CASCADE`, schema))
		return errors.Join(dropErr, admin.Close())
	}
	return database, nil
}
