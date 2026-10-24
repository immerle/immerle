// Package immerle implements the native immerle extension API: the social
// features Subsonic lacks (capability discovery, activity feed, collaborative
// playlists, shares and synchronized Jam sessions).
package immerle

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/api/httputil"
	"github.com/immerle/immerle/internal/api/media"
	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/integrations/lastfm"
	"github.com/immerle/immerle/internal/integrations/listenbrainz"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/platform/logging"
	"github.com/immerle/immerle/internal/services/federation"
	"github.com/immerle/immerle/internal/services/importer"
	"github.com/immerle/immerle/internal/services/ondemand"
	"github.com/immerle/immerle/internal/services/scanner"
	"github.com/immerle/immerle/internal/services/sonic"
	"github.com/immerle/immerle/internal/services/spotifyplaylists"
	"github.com/immerle/immerle/internal/stream"
)

// ProtocolVersion is the immerle extension protocol version.
const ProtocolVersion = "1.0.0"

// Deps holds the immerle handler dependencies.
type Deps struct {
	// Services are the application services shared with the Subsonic handler.
	Services     *core.Services
	Auth         *core.AuthService
	Users        *persistence.UserRepo
	Activity     *core.ActivityService
	Playlists    *persistence.PlaylistRepo
	PlaylistSync core.PlaylistSyncEnqueuer // optional: enqueue public-playlist hub sync
	// ListenBrainz validates a token live when a user sets one on their
	// account (PATCH /me). Optional; nil skips live validation.
	ListenBrainz *listenbrainz.Client
	// LastFm drives the Last.fm desktop-auth handshake (POST /me/lastfm/connect/*).
	// Optional; nil (or the admin not having configured a key/secret) disables it.
	LastFm     *lastfm.Client
	Jam        *core.JamService
	Setup      *core.SetupService
	Federation *federation.Service
	// Catalog and OnDemand enrich activity feed items (titles/cover; OnDemand maps a
	// remote favorite to its downloaded local track). Both also back catalog browse via LibraryService.
	Catalog    *persistence.CatalogRepo
	Genres     *persistence.GenreRepo
	NowPlaying *core.NowPlayingTracker
	OnDemand   *ondemand.CatalogService
	// Streamer and Cover back the media endpoints (audio stream/download, cover art).
	Streamer *stream.Streamer
	Cover    *stream.CoverService
	// BaseURL builds absolute share-link URLs.
	BaseURL string
	// SigningKey signs short-lived media (stream/download) URLs. Empty disables it.
	SigningKey string
	// LibraryStats serves the cached library analytics (counts + total size).
	LibraryStats *core.LibraryStatsService
	// Imports runs playlist imports from external sources (e.g. Spotify).
	Imports *importer.Service
	// SpotifyPlaylists adds public Spotify playlists to the global search when
	// the built-in spotify-playlist provider is enabled, opened as on-demand federated playlists.
	// Optional.
	SpotifyPlaylists *spotifyplaylists.Service
	// Scanner ingests user-uploaded ("local") audio files into the catalog and
	// runs the admin-triggered library scans over LibraryPaths.
	Scanner      *scanner.Scanner
	LibraryPaths []string
	// UploadsDir is where user-uploaded audio files are written (a scanned dir).
	UploadsDir string
	// CoversDir holds cover-art files (custom track covers are written here).
	CoversDir string
	// Cleanup controls the provider-download eviction sweep (admin API).
	Cleanup CleanupController
	// Charts controls the curated chart-playlist sync (admin API).
	Charts ChartsController
	// AutoPlaylists controls the genre/decade auto-playlist sync (admin API).
	AutoPlaylists AutoPlaylistsController
	// Providers manages runtime-configurable on-demand providers (admin API).
	// nil when on-demand is disabled; handlers guard with a nil check.
	Providers *ondemand.ProviderManager
	// Settings manages the DB-backed runtime settings (admin API). It also supplies
	// the device-session JWT lifetime (a runtime setting).
	Settings *core.SettingsService
	// SmartPlaylists persists and evaluates rule-based playlists.
	SmartPlaylists *persistence.SmartPlaylistRepo
	// Radio persists internet radio stations (built-in + custom).
	Radio *persistence.RadioRepo
	// Podcasts manages podcast channels/episodes (feed refresh + downloads).
	Podcasts *core.PodcastService
	// Wrapped computes the per-user year-in-review from the scrobble history.
	Wrapped *persistence.WrappedRepo
	// HallOfFame persists each user's personal top-tracks ranking.
	HallOfFame *persistence.HallOfFameRepo
	// Concerts persists per-user concert-discovery matches. ConcertsSync
	// controls the daily sync (admin API). Implemented by *concerts.Service.
	Concerts     *persistence.ConcertRepo
	ConcertsSync ConcertsController
	// Purchases imports a user's own purchases from external stores (Bandcamp
	// first). nil disables the feature. Implemented by *core.PurchasesService.
	Purchases *core.PurchasesService
	// Sonic runs the local audio analysis (admin progress, capabilities, sonic
	// radio). Optional; nil hides the features.
	Sonic  *sonic.Service
	Logger *slog.Logger
	// LogHub streams live server log lines to the admin log viewer (SSE).
	LogHub *logging.Hub
}

// deviceTokenTTL returns the device-session JWT lifetime from the runtime
// settings (default 30 days when settings are unavailable, e.g. in tests).
func (h *Handler) deviceTokenTTL() time.Duration {
	if h.Settings != nil {
		return h.Settings.DeviceTokenTTL()
	}
	return 720 * time.Hour
}

// CleanupController runs an immediate eviction sweep. The enabled/retention
// state lives in the runtime settings. Implemented by *ondemand.Evictor.
type CleanupController interface {
	Sweep(ctx context.Context) (int, error)
}

// ChartsController runs an immediate curated chart-playlist sync, regardless
// of the weekly schedule. Implemented by *charts.Service.
type ChartsController interface {
	SyncNow(ctx context.Context) (int, error)
}

// AutoPlaylistsController runs an immediate genre/decade auto-playlist sync,
// regardless of the daily schedule. Implemented by *autoplaylists.Service.
type AutoPlaylistsController interface {
	SyncNow(ctx context.Context) (int, error)
}

// ConcertsController runs an immediate concert-discovery sync, regardless of
// the daily schedule. Implemented by *concerts.Service.
type ConcertsController interface {
	SyncNow(ctx context.Context) (int, error)
}

// Handler implements the immerle native API.
type Handler struct {
	Deps
	media *media.Server
}

// NewHandler builds a immerle Handler.
func NewHandler(d Deps) *Handler {
	return &Handler{
		Deps:  d,
		media: media.NewServer(d.Catalog, d.Streamer, d.Cover, d.OnDemand, d.NowPlaying, d.Logger, d.SigningKey),
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
		// Station logos and cover art are cached public images (plain <img>, no
		// Authorization header) — low sensitivity, like the radio logos.
		r.Get("/radio/stations/{id}/cover", h.handleRadioCover)
		r.Get("/cover/{id}", h.handleCover)

		// Audio stream/download accept either a short-lived signed URL (for an
		// <audio>/<video> src that can't send headers) or a Bearer token. The signed
		// URL is a one-track, time-limited capability — no reusable credential in logs/history.
		r.Group(func(r chi.Router) {
			r.Use(h.mediaAuthMiddleware)
			r.Get("/songs/{id}/stream", h.handleStream)
			r.Get("/songs/{id}/download", h.handleDownload)
		})

		// Everything below requires a Bearer token. Each domain mounts its own
		// routes (next to its handlers) on r, and its admin routes on admin.
		r.Group(func(r chi.Router) {
			r.Use(h.authMiddleware)
			r.Route("/admin", func(admin chi.Router) {
				admin.Use(adminOnly)
				for _, mount := range []func(r, admin chi.Router){
					h.accountRoutes,
					h.lastFmRoutes,
					h.purchasesRoutes,
					h.concertsRoutes,
					h.catalogRoutes,
					h.uploadsRoutes,
					h.playlistRoutes,
					h.smartPlaylistRoutes,
					h.importRoutes,
					h.hallOfFameRoutes,
					h.shareRoutes,
					h.playQueueRoutes,
					h.socialRoutes,
					h.radioRoutes,
					h.podcastRoutes,
					h.wrappedRoutes,
					h.serverAdminRoutes,
				} {
					mount(r, admin)
				}
			})
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
