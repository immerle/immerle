// Package immerle implements the native immerle extension API: the social
// features Subsonic lacks (capability discovery, friends, activity feed,
// collaborative playlists, shares and synchronized Jam sessions).
package immerle

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/api/httputil"
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
func (h *Handler) Register(mux chi.Router) {
	// Capability discovery is unauthenticated so apps can detect support.
	mux.HandleFunc("/capabilities", h.handleCapabilities)
	mux.HandleFunc("/rest/immerle.capabilities", h.handleCapabilities)

	// First-run setup is unauthenticated and self-locks once a user exists.
	mux.HandleFunc("/setup/status", h.handleSetupStatus)
	mux.HandleFunc("/setup/init", h.handleSetupInit)

	// Device login is unauthenticated (it exchanges credentials for a JWT).
	mux.HandleFunc("/auth/login", h.handleLogin)

	// Everything below requires authentication; the group middleware applies it
	// once instead of wrapping each handler.
	mux.Group(func(r chi.Router) {
		r.Use(h.authMiddleware)

		r.HandleFunc("/friends", h.handleFriends)
		r.HandleFunc("/friends/request", h.handleFriendRequest)
		r.HandleFunc("/friends/accept", h.handleFriendAccept)
		r.HandleFunc("/friends/pending", h.handleFriendPending)
		r.HandleFunc("/activity", h.handleActivity)
		r.HandleFunc("/profile", h.handleProfile)
		r.HandleFunc("/account", h.handleAccount)
		r.HandleFunc("/library/stats", h.handleLibraryStats)

		// Playlist imports from external sources (e.g. Spotify).
		r.HandleFunc("/imports/sources", h.handleImportSources)
		r.HandleFunc("/imports/start", h.handleImportStart)
		r.HandleFunc("/imports/status", h.handleImportStatus)
		r.HandleFunc("/imports/items/resolve", h.handleImportItemResolve)
		r.HandleFunc("/imports", h.handleImports)
		r.HandleFunc("/playlists/collaborators", h.handleAddCollaborator)
		r.HandleFunc("/playlists/public", h.handlePublicPlaylists)
		r.HandleFunc("/playlists/subscribe", h.handleSubscribePlaylist)
		r.HandleFunc("/playlists/unsubscribe", h.handleUnsubscribePlaylist)
		r.HandleFunc("/jam/create", h.handleJamCreate)
		r.HandleFunc("/jam/join", h.handleJamJoin)
		r.HandleFunc("/jam/leave", h.handleJamLeave)
		r.HandleFunc("/jam/state", h.handleJamState)
		r.HandleFunc("/jam/update", h.handleJamUpdate)
		r.HandleFunc("/jam/events", h.handleJamEvents)

		// Personal API tokens (scoped to the authenticated user).
		r.HandleFunc("/tokens", h.handleTokens)
		r.HandleFunc("/tokens/create", h.handleCreateToken)
		r.HandleFunc("/tokens/revoke", h.handleRevokeToken)

		// Device sessions (JWT): list and revoke.
		r.HandleFunc("/devices", h.handleDevices)
		r.HandleFunc("/devices/revoke", h.handleRevokeDevice)

		// Per-account UI theme (scoped to the authenticated user).
		r.HandleFunc("/theme", h.handleTheme)

		// Admin: runtime control of the provider-download eviction sweep.
		r.HandleFunc("/admin/cleanup", h.handleCleanup)
		r.HandleFunc("/admin/cleanup/run", h.handleCleanupRun)

		// Admin: runtime-configurable on-demand providers.
		r.HandleFunc("/admin/providers", h.handleProviders)
		r.HandleFunc("/admin/providers/enable", h.handleProviderEnable)
		r.HandleFunc("/admin/providers/reorder", h.handleProviderReorder)
		r.HandleFunc("/admin/providers/delete", h.handleProviderDelete)

		// Admin: DB-backed runtime settings (provider behaviour, avatars, scan, federation).
		r.HandleFunc("/admin/settings", h.handleSettings)
	})
}

// authMiddleware authenticates the request (Subsonic-style credentials or a
// device JWT / API token) and injects the user into the context. On failure it
// answers 401 and does not call next.
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		creds := core.Credentials{
			Username:  r.Form.Get("u"),
			Password:  r.Form.Get("p"),
			Token:     r.Form.Get("t"),
			Salt:      r.Form.Get("s"),
			APIToken:  httputil.APITokenFromRequest(r),
			RemoteIP:  httputil.ClientIP(r),
			UserAgent: r.UserAgent(),
		}
		user, err := h.Auth.Authenticate(r.Context(), creds)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized"))
			return
		}
		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
