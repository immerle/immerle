// Package immerle implements the native immerle extension API: the social
// features Subsonic lacks (capability discovery, friends, activity feed,
// collaborative playlists, shares and synchronized Jam sessions).
package immerle

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/api/httputil"
	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/importer"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/scanner"
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
	// (OnDemand maps a remote favorite to its downloaded local track). Catalog,
	// Annotations and OnDemand also back the catalog browse resources via the
	// shared LibraryService.
	Catalog     *persistence.CatalogRepo
	Annotations *persistence.AnnotationRepo
	Genres      *persistence.GenreRepo
	OnDemand    *core.CatalogService
	// LibraryStats serves the cached library analytics (counts + total size).
	LibraryStats *core.LibraryStatsService
	// Imports runs playlist imports from external sources (e.g. Spotify).
	Imports *importer.Service
	// Scanner ingests user-uploaded ("local") audio files into the catalog.
	Scanner *scanner.Scanner
	// UploadsDir is where user-uploaded audio files are written (a scanned dir).
	UploadsDir string
	// CoversDir holds cover-art files (custom track covers are written here).
	CoversDir string
	// Cleanup controls the provider-download eviction sweep (admin API).
	Cleanup CleanupController
	// Providers manages runtime-configurable on-demand providers (admin API).
	// nil when on-demand is disabled; handlers guard with a nil check.
	Providers *core.ProviderManager
	// Settings manages the DB-backed runtime settings (admin API). It also supplies
	// the device-session JWT lifetime (a runtime setting).
	Settings *core.SettingsService
	// SmartPlaylists persists and evaluates rule-based playlists.
	SmartPlaylists *persistence.SmartPlaylistRepo
	// Radio persists internet radio stations (built-in + custom).
	Radio *persistence.RadioRepo
	// Wrapped computes the per-user year-in-review from the scrobble history.
	Wrapped *persistence.WrappedRepo
	Logger  *slog.Logger
}

// deviceTokenTTL returns the device-session JWT lifetime from the runtime
// settings (default 30 days when settings are unavailable, e.g. in tests).
func (h *Handler) deviceTokenTTL() time.Duration {
	if h.Settings != nil {
		return h.Settings.DeviceTokenTTL()
	}
	return 720 * time.Hour
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
	// library backs the catalog browse resources with the same application
	// service the Subsonic handler uses.
	library *core.LibraryService
}

// NewHandler builds a immerle Handler.
func NewHandler(d Deps) *Handler {
	return &Handler{
		Deps:    d,
		library: core.NewLibraryService(d.Catalog, d.Annotations, d.OnDemand),
	}
}

type ctxKey int

const userKey ctxKey = iota

// Register mounts the native immerle REST API under /api/v1. Authentication is
// via "Authorization: Bearer <device-jwt | api-token>" (the Subsonic API keeps
// its own query-param auth). The legacy /rest/immerle.capabilities alias is kept
// for Subsonic-style capability discovery.
func (h *Handler) Register(mux chi.Router) {
	// Legacy capability-discovery alias (Subsonic-style probing).
	mux.Get("/rest/immerle.capabilities", h.handleCapabilities)

	mux.Route("/api/v1", func(r chi.Router) {
		// Public (no auth): discovery, first-run setup, session creation.
		r.Get("/capabilities", h.handleCapabilities)
		r.Get("/setup", h.handleSetupStatus)
		r.Post("/setup", h.handleSetupInit)
		r.Post("/auth/sessions", h.handleLogin)
		// Station logos are cached public images (loadable as a plain <img>).
		r.Get("/radio/stations/{id}/cover", h.handleRadioCover)

		// Everything below requires a Bearer token.
		r.Group(func(r chi.Router) {
			r.Use(h.authMiddleware)

			// Own account / other users' profiles.
			r.Get("/me", h.handleAccount)
			r.Patch("/me", h.handleAccountUpdate)
			r.Get("/users/{username}", h.handleProfile)

			// Friendships.
			r.Get("/friends", h.handleFriends)
			r.Get("/friends/requests", h.handleFriendPending)
			r.Post("/friends/requests", h.handleFriendRequest)
			r.Post("/friends/requests/{username}/accept", h.handleFriendAccept)

			r.Get("/activity", h.handleActivity)
			r.Get("/library/stats", h.handleLibraryStats)

			// Internet radio stations (list + like for everyone; CRUD is admin).
			r.Get("/radio", h.handleRadioList)
			r.Put("/radio/stations/{id}/like", h.handleRadioLike)
			r.Delete("/radio/stations/{id}/like", h.handleRadioUnlike)
			r.Get("/admin/radio", h.handleRadioAdmin)
			r.Put("/admin/radio", h.handleRadioToggle)
			r.Post("/admin/radio/stations", h.handleRadioCreate)
			r.Put("/admin/radio/stations/{id}", h.handleRadioUpdate)
			r.Delete("/admin/radio/stations/{id}", h.handleRadioDelete)

			// Catalog browse over the shared library service.
			r.Get("/artists", h.handleListArtists)
			r.Get("/artists/{id}", h.handleGetArtist)
			r.Get("/albums", h.handleListAlbums)
			r.Get("/albums/{id}", h.handleGetAlbum)
			r.Get("/songs/{id}", h.handleGetSong)
			r.Get("/genres", h.handleGetGenres)
			r.Get("/search", h.handleSearch)

			// Year-in-review ("Wrapped").
			r.Get("/wrapped", h.handleWrapped)

			// "Local" library: tracks the user uploaded from the web UI.
			r.Get("/library/local", h.handleLocalSongs)
			r.Post("/library/uploads", h.handleUpload)
			r.Patch("/library/tracks/{id}", h.handleTrackUpdate)
			r.Put("/library/tracks/{id}/cover", h.handleTrackCover)
			r.Delete("/library/tracks/{id}", h.handleTrackDelete)

			// Playlist imports from external sources (e.g. Spotify).
			r.Get("/imports/sources", h.handleImportSources)
			r.Get("/imports", h.handleImports)
			r.Post("/imports", h.handleImportStart)
			r.Get("/imports/{id}", h.handleImportStatus)
			r.Post("/imports/{id}/items/{itemId}/resolve", h.handleImportItemResolve)

			// Rule-based "smart" playlists.
			r.Get("/smart-playlists", h.handleSmartPlaylists)
			r.Post("/smart-playlists", h.handleSmartPlaylistCreate)
			r.Post("/smart-playlists/preview", h.handleSmartPlaylistPreview)
			r.Put("/smart-playlists/{id}", h.handleSmartPlaylistUpdate)
			r.Delete("/smart-playlists/{id}", h.handleSmartPlaylistDelete)
			r.Get("/smart-playlists/{id}/tracks", h.handleSmartPlaylistTracks)

			// Collaborative / public playlists (extensions over the Subsonic API).
			r.Get("/playlists/public", h.handlePublicPlaylists)
			r.Put("/playlists/{id}/subscription", h.handleSubscribePlaylist)
			r.Delete("/playlists/{id}/subscription", h.handleUnsubscribePlaylist)
			r.Post("/playlists/{id}/collaborators", h.handleAddCollaborator)

			// Synchronized Jam sessions.
			r.Post("/jam", h.handleJamCreate)
			r.Get("/jam/{id}", h.handleJamState)
			r.Patch("/jam/{id}", h.handleJamUpdate)
			r.Get("/jam/{id}/events", h.handleJamEvents)
			r.Post("/jam/{id}/participants", h.handleJamJoin)
			r.Delete("/jam/{id}/participants/me", h.handleJamLeave)

			// Personal API tokens (scoped to the authenticated user).
			r.Get("/tokens", h.handleTokens)
			r.Post("/tokens", h.handleCreateToken)
			r.Delete("/tokens/{id}", h.handleRevokeToken)

			// Device sessions (JWT): list and revoke.
			r.Get("/devices", h.handleDevices)
			r.Delete("/devices/{id}", h.handleRevokeDevice)

			// Per-account UI theme.
			r.Get("/theme", h.handleTheme)
			r.Patch("/theme", h.handleThemeUpdate)

			// Admin: library track management (list, edit, cover, delete).
			r.Get("/admin/tracks", h.handleAdminTracks)
			r.Patch("/admin/tracks/{id}", h.handleAdminTrackUpdate)
			r.Put("/admin/tracks/{id}/cover", h.handleAdminTrackCover)
			r.Delete("/admin/tracks/{id}", h.handleAdminTrackDelete)

			// Admin: provider-download eviction sweep.
			r.Get("/admin/cleanup", h.handleCleanup)
			r.Put("/admin/cleanup", h.handleCleanupUpdate)
			r.Post("/admin/cleanup/runs", h.handleCleanupRun)

			// Admin: runtime-configurable on-demand providers.
			r.Get("/admin/providers", h.handleProviders)
			r.Post("/admin/providers", h.handleProviderUpsert)
			r.Put("/admin/providers/order", h.handleProviderReorder)
			r.Put("/admin/providers/{name}/enabled", h.handleProviderEnable)
			r.Delete("/admin/providers/{name}", h.handleProviderDelete)
			r.Get("/admin/providers/{name}/logs", h.handleProviderLogs)

			// Admin: DB-backed runtime settings.
			r.Get("/admin/settings", h.handleSettings)
			r.Patch("/admin/settings", h.handleSettingsUpdate)

			// Admin: smart-playlists feature toggle.
			r.Get("/admin/smart-playlists", h.handleSmartPlaylistsAdmin)
			r.Put("/admin/smart-playlists", h.handleSmartPlaylistsToggle)

			// Admin: Wrapped feature toggle.
			r.Get("/admin/wrapped", h.handleWrappedAdmin)
			r.Put("/admin/wrapped", h.handleWrappedUpdate)
		})
	})
}

// authMiddleware authenticates the request via a Bearer device JWT or API token
// and injects the user into the context. On failure it answers 401.
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm() // allows the ?apiKey= fallback in APITokenFromRequest
		user, err := h.Auth.Authenticate(r.Context(), core.Credentials{
			APIToken:  httputil.APITokenFromRequest(r),
			RemoteIP:  httputil.ClientIP(r),
			UserAgent: r.UserAgent(),
		})
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
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
