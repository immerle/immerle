package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("expected sqlite default, got %q", cfg.Database.Driver)
	}
}

func TestLoadDotEnvAndRealEnvWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	env := `# bootstrap config
PORT=9999
DATABASE_DSN="custom.db"
LOG_LEVEL=debug
`
	if err := os.WriteFile(path, []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}

	// A real environment variable takes precedence over the .env file.
	t.Setenv("PORT", "7777")
	// loadDotEnv sets process env vars; clean up the ones only in the file.
	t.Cleanup(func() {
		os.Unsetenv("DATABASE_DSN")
		os.Unsetenv("LOG_LEVEL")
	})

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// PORT (bare number) becomes the ":port" listen address; real env wins.
	if cfg.Server.Address != ":7777" {
		t.Errorf("real env PORT should win over .env, got %q", cfg.Server.Address)
	}
	// Values only in the .env file are applied (quotes stripped).
	if cfg.Database.DSN != "custom.db" {
		t.Errorf(".env value lost: %q", cfg.Database.DSN)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf(".env value lost: %q", cfg.Log.Level)
	}
}

func TestValidateRejectsUnknownDriver(t *testing.T) {
	cfg := Default()
	cfg.Database.Driver = "mysql"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for unknown driver")
	}
}
