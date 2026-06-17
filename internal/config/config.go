// Package config loads gossignol-server BOOTSTRAP configuration from the
// environment (and an optional .env file): the few settings that must be known
// before anything starts and need a restart to change — server port, database,
// the optional auth secret, the setup-token gate, logging and library paths.
// Everything else (the on-demand switch & providers and their credentials,
// provider behaviour, transcoding, avatars, scan cadence, CORS, device-token
// TTL, federation) lives in the DB-backed runtime settings and is managed via
// the admin API — not here.
//
// Variables are plain (e.g. PORT, DATABASE_DSN). A .env file with the same
// KEY=VALUE pairs is loaded at startup; real environment variables take
// precedence over the file.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the root bootstrap configuration object.
type Config struct {
	Server   ServerConfig
	Auth     AuthConfig
	Database DatabaseConfig
	Log      LogConfig
	Library  LibraryConfig
}

// ServerConfig holds HTTP server settings. CORS is a runtime setting, not here.
type ServerConfig struct {
	// Address is the listen address (":<PORT>"), derived from the PORT variable.
	Address string
}

// AuthConfig holds bootstrap auth settings. The device-token TTL is a runtime
// setting, not here.
type AuthConfig struct {
	// Secret encrypts stored passwords (required for Subsonic token auth) and
	// signs internal tokens. When empty, a random secret is generated at startup
	// and persisted under the data dir (data/secret).
	Secret string
	// RequireSetupToken gates first-run admin creation behind a printed token.
	RequireSetupToken bool
}

// DatabaseConfig selects and configures the storage backend.
type DatabaseConfig struct {
	Driver          string // "sqlite" (default) or "postgres"
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// LogConfig controls structured logging.
type LogConfig struct {
	Level  string // debug | info | warn | error
	Format string // json | text
}

// LibraryConfig holds the (bootstrap) library locations. Scan-on-start is always
// on; scan cadence/watch are runtime settings.
type LibraryConfig struct {
	Paths   []string
	DataDir string
}

// TranscodeConfig holds transcoding profiles and ffmpeg location.
type TranscodeConfig struct {
	FFmpegPath  string
	FFprobePath string
	CacheDir    string
	Profiles    []TranscodeProfile
}

// TranscodeProfile describes one named output format.
type TranscodeProfile struct {
	Name       string
	Format     string
	BitRate    int
	FFmpegArgs string
}

// FederationConfig configures the optional gossignol-hub connection. It is no
// longer part of the bootstrap Config (it is a runtime setting); the type is
// kept because the federation service consumes it — app builds it from the
// runtime settings.
type FederationConfig struct {
	Enabled bool
	HubURL  string
	// PublicKey → X-Instance-ID header; PrivateKey → Authorization Bearer token.
	PublicKey       string
	PrivateKey      string
	SyncInterval    time.Duration
	ResolveMissing  bool
	ExportScrobbles bool
}

// Default returns a configuration populated with sensible defaults.
func Default() Config {
	return Config{
		Server: ServerConfig{Address: ":4533"},
		Auth:   AuthConfig{Secret: "", RequireSetupToken: false},
		Database: DatabaseConfig{
			Driver:          "sqlite",
			DSN:             "gossignol.db",
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: 0,
		},
		Log:     LogConfig{Level: "info", Format: "text"},
		Library: LibraryConfig{DataDir: "data"},
	}
}

// Load applies a .env file (if present at envPath, or ".env" when envPath is
// empty) followed by environment-variable overrides on top of the defaults.
func Load(envPath string) (Config, error) {
	if envPath == "" {
		envPath = ".env"
	}
	if err := loadDotEnv(envPath); err != nil {
		return Config{}, err
	}

	cfg := Default()
	applyEnv(&cfg)

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate checks that required values are coherent.
func (c Config) Validate() error {
	switch c.Database.Driver {
	case "sqlite", "postgres":
	default:
		return fmt.Errorf("unsupported database driver %q", c.Database.Driver)
	}
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn must be set")
	}
	if c.Server.Address == "" {
		return fmt.Errorf("server.address must be set")
	}
	return nil
}

// loadDotEnv parses KEY=VALUE lines from path into the environment. Missing file
// is not an error. Real environment variables take precedence (a key already set
// in the environment is left untouched).
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open env file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = unquote(val)
		if key == "" {
			continue
		}
		if _, set := os.LookupEnv(key); set {
			continue // real env wins
		}
		_ = os.Setenv(key, val)
	}
	return sc.Err()
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// applyEnv overrides config fields from environment variables.
func applyEnv(c *Config) {
	setPort(&c.Server.Address, "PORT")
	setString(&c.Auth.Secret, "AUTH_SECRET")
	setBool(&c.Auth.RequireSetupToken, "AUTH_REQUIRE_SETUP_TOKEN")
	setString(&c.Database.Driver, "DATABASE_DRIVER")
	setString(&c.Database.DSN, "DATABASE_DSN")
	setString(&c.Log.Level, "LOG_LEVEL")
	setString(&c.Log.Format, "LOG_FORMAT")
	setString(&c.Library.DataDir, "LIBRARY_DATA_DIR")
	if v := os.Getenv("LIBRARY_PATHS"); v != "" {
		c.Library.Paths = splitAndTrim(v)
	}
}

// setPort sets the listen address from a bare port number (e.g. PORT=4533 →
// ":4533"). A leading ":" in the value is tolerated.
func setPort(addr *string, key string) {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		*addr = ":" + strings.TrimPrefix(strings.TrimSpace(v), ":")
	}
}

func setString(dst *string, key string) {
	if v, ok := os.LookupEnv(key); ok {
		*dst = v
	}
}

func setBool(dst *bool, key string) {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			*dst = b
		}
	}
}

func splitAndTrim(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
