package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestOpenSqlite(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "test.db")
	database, err := Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	if database.Dialect != "sqlite" {
		t.Errorf("Dialect = %q, want sqlite", database.Dialect)
	}
	if err := database.Ping(); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestOpenDefaultDriverIsSqlite(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "test.db")
	database, err := Open("", dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()
	if database.Dialect != "sqlite" {
		t.Errorf("Dialect = %q, want sqlite", database.Dialect)
	}
}

func TestOpenUnsupportedDriver(t *testing.T) {
	if _, err := Open("mysql", "whatever"); err == nil {
		t.Fatal("expected error for unsupported driver")
	}
}

// TestMigrate runs every migration up, all the way down, then up again, so a
// dialect-specific statement in either direction fails here instead of at
// startup. Postgres runs only when IMMERLE_TEST_POSTGRES_DSN is set (CI sets it).
func TestMigrate(t *testing.T) {
	dsns := map[string]string{
		"sqlite":   filepath.Join(t.TempDir(), "test.db"),
		"postgres": os.Getenv("IMMERLE_TEST_POSTGRES_DSN"),
	}
	for driver, dsn := range dsns {
		t.Run(driver, func(t *testing.T) {
			if dsn == "" {
				t.Skip("IMMERLE_TEST_POSTGRES_DSN not set")
			}
			database, err := Open(driver, dsn)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer database.Close()

			ctx := context.Background()
			if err := database.Migrate(ctx); err != nil {
				t.Fatalf("Migrate: %v", err)
			}
			if err := goose.DownToContext(ctx, database.DB, "migrations", 0); err != nil {
				t.Fatalf("migrate down: %v", err)
			}
			if err := database.Migrate(ctx); err != nil {
				t.Fatalf("Migrate after down: %v", err)
			}

			var n int
			if err := database.QueryRow("SELECT COUNT(*) FROM tracks").Scan(&n); err != nil {
				t.Fatalf("expected tracks table after migration: %v", err)
			}
			if driver == "postgres" {
				// Millisecond timestamps overflow int4 (#312).
				err := database.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
					WHERE table_schema = current_schema() AND data_type = 'integer'
					  AND table_name <> 'goose_db_version'`).Scan(&n)
				if err != nil || n != 0 {
					t.Fatalf("int4 columns left after migration: %d (%v)", n, err)
				}
			}
		})
	}
}

func TestRebind(t *testing.T) {
	tests := []struct {
		dialect string
		query   string
		want    string
	}{
		{"sqlite", "SELECT * FROM t WHERE a = ? AND b = ?", "SELECT * FROM t WHERE a = ? AND b = ?"},
		{"postgres", "SELECT * FROM t WHERE a = ? AND b = ?", "SELECT * FROM t WHERE a = $1 AND b = $2"},
		{"postgres", "no placeholders", "no placeholders"},
	}
	for _, tt := range tests {
		database := &DB{Dialect: tt.dialect}
		if got := database.Rebind(tt.query); got != tt.want {
			t.Errorf("Rebind(%q) with dialect %q = %q, want %q", tt.query, tt.dialect, got, tt.want)
		}
	}
}
