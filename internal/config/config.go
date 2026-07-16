// Package config loads immerle-server BOOTSTRAP configuration from the
// environment (and an optional .env file): the few settings that must be known
// before anything starts and need a restart to change — server port, database,
// the optional auth secret, the setup-token gate, an optional env-based admin
// bootstrap, logging and library paths.
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
)

// Config is the root bootstrap configuration object.
type Config struct {
	Server   ServerConfig
	Auth     AuthConfig
	Database DatabaseConfig
	Log      LogConfig
	Library  LibraryConfig
	// HubURL is the resolved immerle-hub endpoint: the hardcoded DefaultHubURL,
	// overridden by DEV_IMMERLE_HUB_URL (real env var or .env) for local debugging.
	HubURL string
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
	// Defaults to false (deliberate UX trade-off for non-technical self-hosters).
	// The setup endpoint self-locks once any user exists, so the exposure window
	// is only between first reachability and setup completing — set true (or
	// keep the instance offline until initialized) if that window matters.
	RequireSetupToken bool
	// AdminUsername and AdminPassword, when both set, bootstrap the first admin
	// account at startup instead of waiting for the setup API/UI. Like
	// RequireSetupToken this only ever applies while the server has no users —
	// it's a no-op on every later restart. Either both must be set or neither
	// (see Validate).
	AdminUsername string
	AdminPassword string
}

// DatabaseConfig selects and configures the storage backend.
type DatabaseConfig struct {
	Driver string // "sqlite" (default) or "postgres"
	DSN    string
}

// LogConfig controls structured logging. Output is always JSON.
type LogConfig struct {
	Level string // debug | info | warn | error
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

// DefaultHubURL is the hardcoded immerle-hub endpoint. It is intentionally NOT
// admin-editable; only the DEV_IMMERLE_HUB_URL variable can override it (real
// env var or .env, real env wins) — resolved into Config.HubURL at load.
const DefaultHubURL = "https://hub.immerle.com"

// FederationConfig configures the optional immerle-hub connection. It is no
// longer part of the bootstrap Config (it is a runtime setting); the type is
// kept because the federation service consumes it — app builds it from the
// runtime settings. HubURL is resolved (hardcoded/env), not stored.
type FederationConfig struct {
	HubURL string
	// UserID is the hub owner UUID the operator pastes to claim this instance
	// (used only at bootstrap). InstanceID is the hub-assigned fixed UUID sent as
	// the X-Instance-ID header. PrivateKey is the hub-issued secret Bearer token
	// (returned once at bootstrap). Sqid is the editable, unique hub handle.
	UserID          string
	InstanceID      string
	Sqid            string
	InstanceName    string
	PrivateKey      string
	SyncPlaylists   bool
	ExportScrobbles bool
}

// Default returns a configuration populated with sensible defaults.
func Default() Config {
	return Config{
		Server: ServerConfig{Address: ":4533"},
		Auth:   AuthConfig{Secret: "", RequireSetupToken: false},
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "immerle.db",
		},
		Log:     LogConfig{Level: "info"},
		Library: LibraryConfig{DataDir: "data"},
		HubURL:  DefaultHubURL,
	}
}

// Load applies a .env file (if present at envPath, or ".env" when envPath is
// empty) followed by environment-variable overrides on top of the defaults.
func Load(envPath string) (Config, error) {
	if envPath == "" {
		envPath = ".env"
	}
	dotenv, err := parseDotEnv(envPath)
	if err != nil {
		return Config{}, err
	}

	cfg := Default()
	applyEnv(&cfg, envLookup(dotenv))

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
	// An empty secret is allowed (one is auto-generated at runtime), but an
	// explicitly-set AUTH_SECRET must be long enough to be a meaningful key for
	// token signing and stored-password encryption.
	if s := c.Auth.Secret; s != "" && len(s) < 16 {
		return fmt.Errorf("auth.secret (AUTH_SECRET) must be at least 16 characters")
	}
	if (c.Auth.AdminUsername == "") != (c.Auth.AdminPassword == "") {
		return fmt.Errorf("ADMIN_USERNAME and ADMIN_PASSWORD must both be set, or neither")
	}
	return nil
}

// parseDotEnv parses KEY=VALUE lines from path into a map. A missing file is not
// an error. Crucially it does NOT call os.Setenv: values (notably secrets) stay
// local to config resolution and are never exported to the process environment,
// so child processes (ffmpeg/ffprobe) don't inherit them.
func parseDotEnv(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("open env file %q: %w", path, err)
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
		val := unquote(strings.TrimSpace(line[eq+1:]))
		if key == "" {
			continue
		}
		out[key] = val
	}
	return out, sc.Err()
}

// envLookup resolves a key from the real environment first, then the parsed
// .env map (real env wins). The .env values are never exported, so they don't
// leak to child processes.
func envLookup(dotenv map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		if v, ok := os.LookupEnv(key); ok {
			return v, true
		}
		v, ok := dotenv[key]
		return v, ok
	}
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// applyEnv overrides config fields from the resolved environment (real env then
// .env map, via lookup).
func applyEnv(c *Config, lookup func(string) (string, bool)) {
	setPort(&c.Server.Address, lookup, "PORT")
	setString(&c.Auth.Secret, lookup, "AUTH_SECRET")
	setBool(&c.Auth.RequireSetupToken, lookup, "AUTH_REQUIRE_SETUP_TOKEN")
	setString(&c.Auth.AdminUsername, lookup, "ADMIN_USERNAME")
	setString(&c.Auth.AdminPassword, lookup, "ADMIN_PASSWORD")
	setString(&c.Database.Driver, lookup, "DATABASE_DRIVER")
	setString(&c.Database.DSN, lookup, "DATABASE_DSN")
	setString(&c.Log.Level, lookup, "LOG_LEVEL")
	setString(&c.Library.DataDir, lookup, "LIBRARY_DATA_DIR")
	if v, ok := lookup("LIBRARY_PATHS"); ok && v != "" {
		c.Library.Paths = splitAndTrim(v)
	}
	// Dev-only: point federation at a local hub. Works from a real env var or
	// .env (real env wins, via lookup); ignored when blank.
	if v, ok := lookup("DEV_IMMERLE_HUB_URL"); ok {
		if t := strings.TrimSpace(v); t != "" {
			c.HubURL = t
		}
	}
}

// setPort sets the listen address from a bare port number (e.g. PORT=4533 →
// ":4533"). A leading ":" in the value is tolerated.
func setPort(addr *string, lookup func(string) (string, bool), key string) {
	if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
		*addr = ":" + strings.TrimPrefix(strings.TrimSpace(v), ":")
	}
}

func setString(dst *string, lookup func(string) (string, bool), key string) {
	if v, ok := lookup(key); ok {
		*dst = v
	}
}

func setBool(dst *bool, lookup func(string) (string, bool), key string) {
	if v, ok := lookup(key); ok {
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
