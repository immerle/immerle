package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gossignol/gossignol/internal/testutil"
)

func newSettings(t *testing.T) (*SettingsService, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "configuration.yaml")
	s, err := NewSettingsService(path, "", "", testutil.NewLogger())
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func TestSettingsServiceHotAndRestart(t *testing.T) {
	s, path := newSettings(t)

	// Defaults applied; the file is written; nothing pending a restart.
	if s.Get().Providers.SearchTimeoutSeconds != 6 {
		t.Fatalf("default search timeout = %d", s.Get().Providers.SearchTimeoutSeconds)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	if len(s.PendingRestart()) != 0 {
		t.Fatalf("nothing should be pending at boot: %v", s.PendingRestart())
	}

	// A hot-reloadable change (search timeout) applies live, no restart.
	next := s.Get()
	next.Providers.SearchTimeoutSeconds = 12
	saved, pending, err := s.Update(next)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Providers.SearchTimeoutSeconds != 12 || len(pending) != 0 {
		t.Fatalf("hot change should not need restart: saved=%+v pending=%v", saved.Providers, pending)
	}
	if s.SearchTimeout() != 12*time.Second {
		t.Fatal("live getter did not reflect the update")
	}

	// Federation is now hot-reloadable too: enabling it needs no restart.
	next = s.Get()
	next.Federation.Enabled = true
	next.Federation.HubURL = "https://hub.test"
	if _, pending, _ = s.Update(next); len(pending) != 0 {
		t.Fatalf("federation changes should be hot, got pending=%v", pending)
	}

	// A restart-required change (toggling the scan watcher) is persisted but
	// flagged pending.
	next = s.Get()
	next.Scan.Watch = !next.Scan.Watch
	wantWatch := next.Scan.Watch
	if _, pending, _ = s.Update(next); len(pending) == 0 {
		t.Fatal("toggling the scan watcher should require a restart")
	}

	// Reloading from the file yields the persisted values, and a fresh boot
	// snapshot means nothing is pending anymore. The YAML is human-readable.
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "searchTimeoutSeconds") {
		t.Fatalf("expected camelCase YAML keys, got:\n%s", b)
	}
	s2, err := NewSettingsService(path, "", "", testutil.NewLogger())
	if err != nil {
		t.Fatal(err)
	}
	if s2.Get().Scan.Watch != wantWatch || s2.Get().Providers.SearchTimeoutSeconds != 12 {
		t.Fatalf("settings not persisted: %+v", s2.Get())
	}
	if len(s2.PendingRestart()) != 0 {
		t.Fatalf("a fresh boot should have nothing pending: %v", s2.PendingRestart())
	}
}

func TestSettingsServiceSanitizes(t *testing.T) {
	s, _ := newSettings(t)

	next := s.Get()
	next.Providers.SearchTimeoutSeconds = 0 // invalid → clamped to default
	saved, _, _ := s.Update(next)
	if saved.Providers.SearchTimeoutSeconds != 6 {
		t.Fatalf("timeout not clamped: %d", saved.Providers.SearchTimeoutSeconds)
	}
}

func TestSettingsServiceSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "configuration.yaml")

	// First run generates and persists a secret in the YAML.
	s, err := NewSettingsService(path, "", "", testutil.NewLogger())
	if err != nil {
		t.Fatal(err)
	}
	secret := s.Secret()
	if secret == "" {
		t.Fatal("secret not generated")
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), secret) {
		t.Fatal("secret not persisted in the config file")
	}

	// A second boot reuses the same secret.
	s2, _ := NewSettingsService(path, "", "", testutil.NewLogger())
	if s2.Secret() != secret {
		t.Fatal("secret not stable across boots")
	}

	// An env secret overrides the file.
	s3, _ := NewSettingsService(path, "env-secret", "", testutil.NewLogger())
	if s3.Secret() != "env-secret" {
		t.Fatalf("env secret should win, got %q", s3.Secret())
	}

	// A legacy data/secret file is migrated when the config has none.
	dir2 := t.TempDir()
	legacy := filepath.Join(dir2, "secret")
	_ = os.WriteFile(legacy, []byte("legacy-secret\n"), 0o600)
	s4, _ := NewSettingsService(filepath.Join(dir2, "configuration.yaml"), "", legacy, testutil.NewLogger())
	if s4.Secret() != "legacy-secret" {
		t.Fatalf("legacy secret not migrated, got %q", s4.Secret())
	}
}
