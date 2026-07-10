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

func TestHubURLOverride(t *testing.T) {
	// Default: the hardcoded hub URL.
	if got := Default().HubURL; got != DefaultHubURL {
		t.Fatalf("default hub URL = %q, want %q", got, DefaultHubURL)
	}

	// DEV_IMMERLE_HUB_URL from the .env file overrides the hardcoded default.
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("DEV_IMMERLE_HUB_URL=http://localhost:8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Unsetenv("DEV_IMMERLE_HUB_URL") })
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HubURL != "http://localhost:8080" {
		t.Fatalf(".env hub override lost: %q", cfg.HubURL)
	}

	// A real environment variable takes precedence over the .env file.
	t.Setenv("DEV_IMMERLE_HUB_URL", "http://127.0.0.1:9090")
	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HubURL != "http://127.0.0.1:9090" {
		t.Fatalf("real env hub override should win, got %q", cfg.HubURL)
	}
}

func TestValidateRejectsUnknownDriver(t *testing.T) {
	cfg := Default()
	cfg.Database.Driver = "mysql"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for unknown driver")
	}
}

func TestAdminEnvVars(t *testing.T) {
	t.Setenv("ADMIN_USERNAME", "kilian")
	t.Setenv("ADMIN_PASSWORD", "password123")

	cfg, err := Load(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Auth.AdminUsername != "kilian" || cfg.Auth.AdminPassword != "password123" {
		t.Fatalf("admin env vars not applied, got %+v", cfg.Auth)
	}
}

func TestValidateRejectsPartialAdminEnv(t *testing.T) {
	cfg := Default()
	cfg.Auth.AdminUsername = "kilian"
	// AdminPassword left empty.
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when only ADMIN_USERNAME is set")
	}

	cfg = Default()
	cfg.Auth.AdminPassword = "password123"
	// AdminUsername left empty.
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when only ADMIN_PASSWORD is set")
	}
}
