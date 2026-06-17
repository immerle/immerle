// Package immerle implements the native immerle extension API: the social
// features Subsonic lacks (capability discovery, friends, activity feed,
// collaborative playlists, shares and synchronized Jam sessions).
package immerle

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/importer"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// ProtocolVersion is the immerle extension protocol version.
const ProtocolVersion = "1.0.0"

// Deps holds the immerle handler dependencies.
type Deps struct {
	Auth       *core.AuthService
	Users      *persistence.UserRepo
	Friends    *persistence.FriendRepo
	Activity   *core.ActivityService
	Playlists  *persistence.PlaylistRepo
	Jam        *core.JamService
	Setup      *core.SetupService
	Federation FederationStatusProvider
	// Catalog and OnDemand enrich activity feed items with titles/cover/etc.
	// (OnDemand maps a remote favorite to its downloaded local track).
	Catalog  *persistence.CatalogRepo
	OnDemand *core.CatalogService
	// LibraryStats serves the cached library analytics (counts + total size).
	LibraryStats *core.LibraryStatsService
	// Imports runs playlist imports from external sources (e.g. Spotify).
	Imports *importer.Service
	// Cleanup controls the provider-download eviction sweep (admin API).
	Cleanup CleanupController
	// Providers manages runtime-configurable on-demand providers (admin API).
	Providers ProviderController
	// Settings manages the DB-backed runtime settings (admin API). It also supplies
	// the device-session JWT lifetime (a runtime setting).
	Settings *core.SettingsService
	Logger   *slog.Logger
}

// deviceTokenTTL returns the device-session JWT lifetime from the runtime
// settings (default 30 days when settings are unavailable, e.g. in tests).
func (h *Handler) deviceTokenTTL() time.Duration {
	if h.Settings != nil {
		return h.Settings.DeviceTokenTTL()
	}
	return 720 * time.Hour
}

// ProviderController manages admin-configurable on-demand providers at runtime.
// Implemented by *core.ProviderManager.
type ProviderController interface {
	List(ctx context.Context) ([]models.ProviderConfig, error)
	Upsert(ctx context.Context, cfg models.ProviderConfig) (models.ProviderConfig, error)
	SetEnabled(ctx context.Context, name string, enabled bool) (models.ProviderConfig, error)
	Reorder(ctx context.Context, names []string) error
	Delete(ctx context.Context, name string) error
	Active(name string) bool
}

// FederationStatusProvider reports whether hub federation is enabled (S7).
type FederationStatusProvider interface {
	Enabled() bool
}

// CleanupController runs an immediate eviction sweep. The enabled/retention
// state lives in the runtime settings. Implemented by *core.Evictor.
type CleanupController interface {
	Sweep(ctx context.Context) (int, error)
}

// Handler implements the immerle native API.
type Handler struct {
	Deps
}

// NewHandler builds a immerle Handler.
func NewHandler(d Deps) *Handler { return &Handler{Deps: d} }

type ctxKey int

const userKey ctxKey = iota

// Register mounts the native immerle extension endpoints on mux at the root
// (the Subsonic API lives under /rest/). The legacy /rest/immerle.capabilities
// alias is kept for Subsonic-style capability discovery.
func (h *Handler) Register(mux *http.ServeMux) {
	// Capability discovery is unauthenticated so apps can detect support.
	mux.HandleFunc("/capabilities", h.handleCapabilities)
	mux.HandleFunc("/rest/immerle.capabilities", h.handleCapabilities)

	// First-run setup is unauthenticated and self-locks once a user exists.
	mux.HandleFunc("/setup/status", h.handleSetupStatus)
	mux.HandleFunc("/setup/init", h.handleSetupInit)

	// Device login is unauthenticated (it exchanges credentials for a JWT).
	mux.HandleFunc("/auth/login", h.handleLogin)

	auth := func(fn http.HandlerFunc) http.HandlerFunc { return h.authenticated(fn) }
	mux.Handle("/friends", auth(h.handleFriends))
	mux.Handle("/friends/request", auth(h.handleFriendRequest))
	mux.Handle("/friends/accept", auth(h.handleFriendAccept))
	mux.Handle("/friends/pending", auth(h.handleFriendPending))
	mux.Handle("/activity", auth(h.handleActivity))
	mux.Handle("/profile", auth(h.handleProfile))
	mux.Handle("/account", auth(h.handleAccount))
	mux.Handle("/library/stats", auth(h.handleLibraryStats))

	// Playlist imports from external sources (e.g. Spotify).
	mux.Handle("/imports/sources", auth(h.handleImportSources))
	mux.Handle("/imports/start", auth(h.handleImportStart))
	mux.Handle("/imports/status", auth(h.handleImportStatus))
	mux.Handle("/imports/items/resolve", auth(h.handleImportItemResolve))
	mux.Handle("/imports", auth(h.handleImports))
	mux.Handle("/playlists/collaborators", auth(h.handleAddCollaborator))
	mux.Handle("/playlists/public", auth(h.handlePublicPlaylists))
	mux.Handle("/playlists/subscribe", auth(h.handleSubscribePlaylist))
	mux.Handle("/playlists/unsubscribe", auth(h.handleUnsubscribePlaylist))
	mux.Handle("/jam/create", auth(h.handleJamCreate))
	mux.Handle("/jam/join", auth(h.handleJamJoin))
	mux.Handle("/jam/leave", auth(h.handleJamLeave))
	mux.Handle("/jam/state", auth(h.handleJamState))
	mux.Handle("/jam/update", auth(h.handleJamUpdate))
	mux.Handle("/jam/events", auth(h.handleJamEvents))

	// Personal API tokens (scoped to the authenticated user).
	mux.Handle("/tokens", auth(h.handleTokens))
	mux.Handle("/tokens/create", auth(h.handleCreateToken))
	mux.Handle("/tokens/revoke", auth(h.handleRevokeToken))

	// Device sessions (JWT): list and revoke.
	mux.Handle("/devices", auth(h.handleDevices))
	mux.Handle("/devices/revoke", auth(h.handleRevokeDevice))

	// Per-account UI theme (scoped to the authenticated user).
	mux.Handle("/theme", auth(h.handleTheme))

	// Admin: runtime control of the provider-download eviction sweep.
	mux.Handle("/admin/cleanup", auth(h.handleCleanup))
	mux.Handle("/admin/cleanup/run", auth(h.handleCleanupRun))

	// Admin: runtime-configurable on-demand providers.
	mux.Handle("/admin/providers", auth(h.handleProviders))
	mux.Handle("/admin/providers/enable", auth(h.handleProviderEnable))
	mux.Handle("/admin/providers/reorder", auth(h.handleProviderReorder))
	mux.Handle("/admin/providers/delete", auth(h.handleProviderDelete))

	// Admin: DB-backed runtime settings (provider behaviour, avatars, scan, federation).
	mux.Handle("/admin/settings", auth(h.handleSettings))
}

// apiTokenFromRequest extracts a personal API token / device JWT from the
// Authorization Bearer header or the apiKey parameter. r.ParseForm must have
// been called.
func apiTokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return r.Form.Get("apiKey")
}

// clientIP returns the best-effort client IP.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (h *Handler) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		creds := core.Credentials{
			Username:  r.Form.Get("u"),
			Password:  r.Form.Get("p"),
			Token:     r.Form.Get("t"),
			Salt:      r.Form.Get("s"),
			APIToken:  apiTokenFromRequest(r),
			RemoteIP:  clientIP(r),
			UserAgent: r.UserAgent(),
		}
		user, err := h.Auth.Authenticate(r.Context(), creds)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized"))
			return
		}
		ctx := context.WithValue(r.Context(), userKey, user)
		next(w, r.WithContext(ctx))
	}
}

func userFrom(ctx context.Context) models.User {
	u, _ := ctx.Value(userKey).(models.User)
	return u
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func errorBody(msg string) map[string]any {
	return map[string]any{"ok": false, "error": msg}
}

func okBody(data map[string]any) map[string]any {
	if data == nil {
		data = map[string]any{}
	}
	data["ok"] = true
	return data
}
