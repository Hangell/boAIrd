package storage

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

type databaseDialect string

const (
	dialectSQLite   databaseDialect = "sqlite"
	dialectPostgres databaseDialect = "postgres"
)

// Database keeps repository code independent from driver-specific SQL syntax.
type Database struct {
	connection *sql.DB
	dialect    databaseDialect
	cleanup    func() error
}

func (d *Database) Close() error {
	closeErr := d.connection.Close()
	if d.cleanup == nil {
		return closeErr
	}
	return errors.Join(closeErr, d.cleanup())
}

func (d *Database) PingContext(ctx context.Context) error {
	return d.connection.PingContext(ctx)
}

func (d *Database) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	query = d.bind(query)
	args = d.bindArgs(args)
	return d.connection.ExecContext(ctx, query, args...)
}

func (d *Database) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	query = d.bind(query)
	args = d.bindArgs(args)
	return d.connection.QueryContext(ctx, query, args...)
}

func (d *Database) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.connection.QueryRowContext(ctx, d.bind(query), d.bindArgs(args)...)
}

func (d *Database) Exec(query string, args ...any) (sql.Result, error) {
	return d.ExecContext(context.Background(), query, args...)
}

func (d *Database) Query(query string, args ...any) (*sql.Rows, error) {
	return d.QueryContext(context.Background(), query, args...)
}

func (d *Database) QueryRow(query string, args ...any) *sql.Row {
	return d.QueryRowContext(context.Background(), query, args...)
}

func (d *Database) BeginTx(ctx context.Context, options *sql.TxOptions) (*Transaction, error) {
	tx, err := d.connection.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &Transaction{transaction: tx, dialect: d.dialect}, nil
}

func (d *Database) bind(query string) string {
	if d.dialect == dialectSQLite {
		return query
	}
	return bindPostgres(query)
}

func (d *Database) bindArgs(args []any) []any {
	if d.dialect == dialectSQLite {
		return args
	}
	return bindPostgresArgs(args)
}

// Transaction applies the same dialect rules as its parent connection.
type Transaction struct {
	transaction *sql.Tx
	dialect     databaseDialect
}

func (t *Transaction) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	query = bindForDialect(t.dialect, query)
	args = bindArgsForDialect(t.dialect, args)
	return t.transaction.ExecContext(ctx, query, args...)
}

func (t *Transaction) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	query = bindForDialect(t.dialect, query)
	args = bindArgsForDialect(t.dialect, args)
	return t.transaction.QueryContext(ctx, query, args...)
}

func (t *Transaction) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.transaction.QueryRowContext(ctx, bindForDialect(t.dialect, query), bindArgsForDialect(t.dialect, args)...)
}

func (t *Transaction) Commit() error   { return t.transaction.Commit() }
func (t *Transaction) Rollback() error { return t.transaction.Rollback() }

func bindForDialect(dialect databaseDialect, query string) string {
	if dialect == dialectSQLite {
		return query
	}
	return bindPostgres(query)
}

func bindArgsForDialect(dialect databaseDialect, args []any) []any {
	if dialect == dialectSQLite {
		return args
	}
	return bindPostgresArgs(args)
}

func bindPostgresArgs(args []any) []any {
	var converted []any
	for index, argument := range args {
		value, ok := argument.(bool)
		if !ok {
			if converted != nil {
				converted[index] = argument
			}
			continue
		}
		if converted == nil {
			converted = append([]any(nil), args...)
		}
		if value {
			converted[index] = int32(1)
		} else {
			converted[index] = int32(0)
		}
	}
	if converted == nil {
		return args
	}
	return converted
}

func bindPostgres(query string) string {
	var result strings.Builder
	result.Grow(len(query) + 16)
	argument := 0
	state := byte(0)
	for i := 0; i < len(query); i++ {
		current := query[i]
		switch state {
		case '\'':
			result.WriteByte(current)
			if current == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					i++
					result.WriteByte(query[i])
				} else {
					state = 0
				}
			}
		case '"':
			result.WriteByte(current)
			if current == '"' {
				state = 0
			}
		default:
			if current == '-' && i+1 < len(query) && query[i+1] == '-' {
				result.WriteString("--")
				i++
				for i+1 < len(query) && query[i+1] != '\n' {
					i++
					result.WriteByte(query[i])
				}
				continue
			}
			if state == 0 && hasCurrentTimestamp(query, i) {
				result.WriteString("to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"')")
				i += len("CURRENT_TIMESTAMP") - 1
				continue
			}
			switch current {
			case '\'':
				state = '\''
				result.WriteByte(current)
			case '"':
				state = '"'
				result.WriteByte(current)
			case '?':
				argument++
				result.WriteByte('$')
				result.WriteString(strconv.Itoa(argument))
			default:
				result.WriteByte(current)
			}
		}
	}
	return result.String()
}

func hasCurrentTimestamp(query string, offset int) bool {
	const keyword = "CURRENT_TIMESTAMP"
	if offset+len(keyword) > len(query) || !strings.EqualFold(query[offset:offset+len(keyword)], keyword) {
		return false
	}
	if offset > 0 && isSQLIdentifier(query[offset-1]) {
		return false
	}
	if offset+len(keyword) < len(query) && isSQLIdentifier(query[offset+len(keyword)]) {
		return false
	}
	return true
}

func isSQLIdentifier(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}
