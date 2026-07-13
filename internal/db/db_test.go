package db

import (
	"context"
	"path/filepath"
	"testing"
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

func TestMigrate(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "test.db")
	database, err := Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var name string
	if err := database.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='tracks'").Scan(&name); err != nil {
		t.Fatalf("expected tracks table after migration: %v", err)
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
