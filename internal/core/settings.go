package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/immerle/immerle/internal/models"
)

// Configuration is the on-disk document (data/configuration.yaml): the auth
// secret plus all admin-managed runtime settings.
type Configuration struct {
	// Secret signs tokens / encrypts stored passwords. Auto-generated if empty.
	Secret   string                 `json:"secret"`
	Settings models.RuntimeSettings `json:"settings"`
}

// SettingsService owns the YAML configuration file: it loads it at boot (seeding
// defaults + generating the secret on first run), serves the current values
// (some read live for hot-reload), and writes updates back to disk atomically.
// Settings a running process can't apply live (avatars, the file watcher,
// transcoding, federation) are flagged as needing a restart instead.
type SettingsService struct {
	path   string
	logger *slog.Logger

	mu      sync.RWMutex
	secret  string
	current models.RuntimeSettings
	boot    models.RuntimeSettings // snapshot at process start, for restart detection
}

// NewSettingsService loads (or seeds) the configuration file at path. envSecret,
// if set, overrides the file secret; legacySecretPath is an older data/secret
// file migrated into the config when present.
func NewSettingsService(path, envSecret, legacySecretPath string, logger *slog.Logger) (*SettingsService, error) {
	s := &SettingsService{path: path, logger: logger}

	cfg := Configuration{Settings: models.DefaultRuntimeSettings()}
	if raw, err := os.ReadFile(path); err == nil {
		if err := yamlToJSON(raw, &cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	cfg.Settings = sanitizeSettings(cfg.Settings)

	// Resolve the secret: env override > file > legacy data/secret > generate.
	secret := strings.TrimSpace(envSecret)
	if secret == "" {
		secret = strings.TrimSpace(cfg.Secret)
	}
	if secret == "" && legacySecretPath != "" {
		if b, err := os.ReadFile(legacySecretPath); err == nil {
			secret = strings.TrimSpace(string(b))
		}
	}
	if secret == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		secret = hex.EncodeToString(buf)
		logger.Info("generated a new auth secret", "file", path)
	}
	cfg.Secret = secret

	s.secret = secret
	s.current = cfg.Settings
	s.boot = cfg.Settings
	if err := s.write(cfg); err != nil { // persist (seed file / secret / new fields)
		return nil, err
	}
	return s, nil
}

// Secret returns the auth secret.
func (s *SettingsService) Secret() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.secret
}

// Get returns a copy of the current settings.
func (s *SettingsService) Get() models.RuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Update persists and applies new settings, returning the stored values and the
// list of fields that changed but only take effect after a restart.
func (s *SettingsService) Update(next models.RuntimeSettings) (models.RuntimeSettings, []string, error) {
	next = sanitizeSettings(next)
	// Hold the lock across persist + assign so a racing Update can't leave the
	// in-memory current out of sync with what was written to disk.
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.write(Configuration{Secret: s.secret, Settings: next}); err != nil {
		return next, nil, err
	}
	s.current = next
	return next, s.pendingRestartLocked(), nil
}

// PendingRestart lists the restart-only fields whose current value differs from
// the value active since boot.
func (s *SettingsService) PendingRestart() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingRestartLocked()
}

// pendingRestartLocked computes the pending-restart fields; callers must hold
// s.mu (read or write).
func (s *SettingsService) pendingRestartLocked() []string {
	cur, boot := s.current, s.boot
	var out []string
	if !transcodeEqual(cur.Transcode, boot.Transcode) {
		out = append(out, "transcode")
	}
	if cur.Scan.Watch != boot.Scan.Watch {
		out = append(out, "scan.watch")
	}
	return out
}

// write serializes cfg to the YAML file atomically (temp + rename).
func (s *SettingsService) write(cfg Configuration) error {
	data, err := jsonToYAML(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// jsonToYAML marshals v to YAML using its json field names (so the file mirrors
// the admin API), via a json round-trip.
func jsonToYAML(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return yaml.Marshal(m)
}

// yamlToJSON unmarshals YAML into v using its json field names.
func yamlToJSON(data []byte, v any) error {
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// transcodeEqual compares two transcode settings (slices included).
func transcodeEqual(a, b models.TranscodeRuntime) bool {
	return a.FFmpegPath == b.FFmpegPath && a.FFprobePath == b.FFprobePath &&
		slices.Equal(a.Profiles, b.Profiles)
}

// sanitizeSettings clamps/normalizes incoming values.
func sanitizeSettings(rs models.RuntimeSettings) models.RuntimeSettings {
	if rs.Providers.SearchTimeoutSeconds <= 0 {
		rs.Providers.SearchTimeoutSeconds = 3
	}
	if rs.Scan.IntervalSeconds < 0 {
		rs.Scan.IntervalSeconds = 0
	}
	if rs.Federation.SyncIntervalSeconds <= 0 {
		rs.Federation.SyncIntervalSeconds = 3600
	}
	if rs.Auth.DeviceTokenTTLSeconds < 0 {
		rs.Auth.DeviceTokenTTLSeconds = 0
	}
	if rs.Transcode.FFmpegPath == "" {
		rs.Transcode.FFmpegPath = "ffmpeg"
	}
	if rs.Transcode.FFprobePath == "" {
		rs.Transcode.FFprobePath = "ffprobe"
	}
	if len(rs.Server.CORSAllowedOrigins) == 0 {
		rs.Server.CORSAllowedOrigins = []string{"*"}
	}
	if rs.Cleanup.MaxAgeSeconds <= 0 {
		rs.Cleanup.MaxAgeSeconds = 720 * 3600
	}
	if rs.Cleanup.IntervalSeconds <= 0 {
		rs.Cleanup.IntervalSeconds = 6 * 3600
	}
	return rs
}

// --- live getters consumed by hot-reloadable components ---

// AutoDownloadOnPlay reports whether a remote play should download.
func (s *SettingsService) AutoDownloadOnPlay() bool { return s.Get().Providers.AutoDownloadOnPlay }

// SearchTimeout returns the remote-search timeout.
func (s *SettingsService) SearchTimeout() time.Duration {
	return time.Duration(s.Get().Providers.SearchTimeoutSeconds) * time.Second
}

// ScanInterval returns the periodic rescan interval (0 disables).
func (s *SettingsService) ScanInterval() time.Duration {
	return time.Duration(s.Get().Scan.IntervalSeconds) * time.Second
}

// ImportSources returns the per-source import config (keyed by source name),
// read live so credential edits apply without a restart.
func (s *SettingsService) ImportSources() map[string]map[string]string {
	return s.Get().Import.Sources
}

// DeviceTokenTTL returns the device-session JWT lifetime (0 = never expires).
func (s *SettingsService) DeviceTokenTTL() time.Duration {
	return time.Duration(s.Get().Auth.DeviceTokenTTLSeconds) * time.Second
}

// CORSOrigins returns the CORS allowed origins (read live by the middleware).
func (s *SettingsService) CORSOrigins() []string {
	return s.Get().Server.CORSAllowedOrigins
}

// CleanupEnabled reports whether the eviction sweep is on (read live).
func (s *SettingsService) CleanupEnabled() bool { return s.Get().Cleanup.Enabled }

// CleanupMaxAge returns the unplayed retention window (read live).
func (s *SettingsService) CleanupMaxAge() time.Duration {
	return time.Duration(s.Get().Cleanup.MaxAgeSeconds) * time.Second
}

// CleanupInterval returns the sweep cadence (used at boot).
func (s *SettingsService) CleanupInterval() time.Duration {
	return time.Duration(s.Get().Cleanup.IntervalSeconds) * time.Second
}
