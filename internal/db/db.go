// Package db provides the database connection, pooling, migrations and helpers
// shared by the persistence layer. SQLite is the default backend; Postgres is
// supported for larger deployments.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strconv"
	"time"

	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib" // postgres driver
	_ "modernc.org/sqlite"             // pure-Go sqlite driver
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// DB wraps *sql.DB and remembers the dialect for portable query building.
type DB struct {
	*sql.DB
	Dialect string
}

// Open opens a database connection pool for the given driver and DSN.
// driver is "sqlite" or "postgres".
func Open(driver, dsn string) (*DB, error) {
	sqlDriver, connStr, dialect, err := resolveDriver(driver, dsn)
	if err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open(sqlDriver, connStr)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Single connection: the SQLite file backend serializes writes, and the
	// app's access patterns don't benefit from a larger pool.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if dialect == "sqlite" {
		// Enable foreign keys and a sane busy timeout for the file backend.
		for _, pragma := range []string{
			"PRAGMA foreign_keys = ON",
			"PRAGMA journal_mode = WAL",
			"PRAGMA busy_timeout = 5000",
		} {
			if _, err := sqlDB.ExecContext(ctx, pragma); err != nil {
				return nil, fmt.Errorf("apply %q: %w", pragma, err)
			}
		}
	}

	return &DB{DB: sqlDB, Dialect: dialect}, nil
}

func resolveDriver(driver, dsn string) (sqlDriver, connStr, dialect string, err error) {
	switch driver {
	case "sqlite", "sqlite3", "":
		// modernc sqlite benefits from _pragma busy timeout via DSN too.
		return "sqlite", dsn, "sqlite", nil
	case "postgres", "pgx":
		return "pgx", dsn, "postgres", nil
	default:
		return "", "", "", fmt.Errorf("unsupported database driver %q", driver)
	}
}

// Migrate applies all pending migrations using goose.
func (db *DB) Migrate(ctx context.Context) error {
	goose.SetBaseFS(migrationFS)
	if err := goose.SetDialect(db.Dialect); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db.DB, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// Rebind converts '?' placeholders to the dialect's positional form. SQLite uses
// '?' as-is; Postgres requires $1, $2, ...
func (db *DB) Rebind(query string) string {
	if db.Dialect != "postgres" {
		return query
	}
	return rebindPositional(query)
}

func rebindPositional(query string) string {
	out := make([]byte, 0, len(query)+8)
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			out = append(out, '$')
			out = strconv.AppendInt(out, int64(n), 10)
			continue
		}
		out = append(out, query[i])
	}
	return string(out)
}
