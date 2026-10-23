package storage

import (
	"strings"
	"testing"
)

func TestBindPostgresRewritesOnlyPositionalPlaceholders(t *testing.T) {
	query := `SELECT ?, '?', "?" -- ?
FROM records WHERE id = ? AND created_at < CURRENT_TIMESTAMP`
	want := `SELECT $1, '?', "?" -- ?
FROM records WHERE id = $2 AND created_at < to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`
	if got := bindPostgres(query); got != want {
		t.Fatalf("bindPostgres() = %q, want %q", got, want)
	}
}

func TestSQLiteBindingPreservesQuestionMarkPlaceholders(t *testing.T) {
	query := `SELECT ?, ?`
	if got := bindForDialect(dialectSQLite, query); got != query {
		t.Fatalf("bindForDialect() = %q, want %q", got, query)
	}
}

func TestBindPostgresArgsConvertsSQLiteBooleanValues(t *testing.T) {
	arguments := []any{"project", true, false}
	converted := bindPostgresArgs(arguments)
	if converted[0] != "project" || converted[1] != int32(1) || converted[2] != int32(0) {
		t.Fatalf("bindPostgresArgs() = %#v, want string and int32 boolean values", converted)
	}
	if _, ok := arguments[1].(bool); !ok {
		t.Fatal("bindPostgresArgs() mutated the caller's arguments")
	}
}

func TestSplitPostgresStatementsPreservesFunctionBodiesAndComments(t *testing.T) {
	script := `-- migration header;
CREATE TABLE records (id TEXT);
/* semicolon ; in block comment */
CREATE FUNCTION example() RETURNS void LANGUAGE plpgsql AS $fn$
BEGIN
    PERFORM ';';
END;
$fn$;
INSERT INTO records(id) VALUES ('value;still-value');`

	statements := splitPostgresStatements(script)
	if len(statements) != 3 {
		t.Fatalf("statement count = %d, want 3: %#v", len(statements), statements)
	}
	if !strings.Contains(statements[1], "PERFORM ';';") || !strings.Contains(statements[2], "value;still-value") {
		t.Fatalf("statement bodies were split incorrectly: %#v", statements)
	}
}
