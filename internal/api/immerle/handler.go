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

		// Everything below requires a Bearer token.
		r.Group(func(r chi.Router) {
			r.Use(h.authMiddleware)

			// Own account / other users' profiles.
			r.Get("/me", h.handleAccount)
			r.Patch("/me", h.handleAccountUpdate)
			r.Get("/me/favorites", h.handleFavorites)
			r.Get("/me/custom-playlists", h.handleCustomPlaylists)
			r.Get("/me/discover-playlists", h.handleDiscoverPlaylists)
			r.Get("/me/concerts", h.handleMyConcerts)
			r.Put("/me/concerts/{id}/dismiss", h.handleDismissConcert)
			r.Get("/me/lastfm", h.handleLastFmStatus)
			r.Post("/me/lastfm/connect/start", h.handleLastFmConnectStart)
			r.Post("/me/lastfm/connect/finish", h.handleLastFmConnectFinish)
			r.Delete("/me/lastfm", h.handleLastFmDisconnect)
			r.Get("/me/purchases/bandcamp", h.handleBandcampStatus)
			r.Post("/me/purchases/bandcamp/connect", h.handleBandcampConnect)
			r.Delete("/me/purchases/bandcamp", h.handleBandcampDisconnect)
			r.Get("/me/purchases/bandcamp/collection", h.handleBandcampCollection)
			r.Post("/me/purchases/bandcamp/items/{saleItemType}/{saleItemId}/import", h.handleBandcampImport)
			r.Get("/me/purchases/bandcamp/jobs", h.handleBandcampJobs)
			r.Put("/me/password", h.handleChangePassword)
			r.Get("/users", h.handleListPublicUsers)
			r.Get("/users/{username}", h.handleProfile)
			r.Get("/users/{username}/hall-of-fame", h.handleUserHallOfFame)

			r.Get("/activity", h.handleActivity)
			r.Get("/library/stats", h.handleLibraryStats)

			// Internet radio stations (list + like for everyone; CRUD is admin).
			r.Get("/radio", h.handleRadioList)
			r.Put("/radio/stations/{id}/like", h.handleRadioLike)
			r.Delete("/radio/stations/{id}/like", h.handleRadioUnlike)

			// Podcasts: browse + download for everyone; channel CRUD/refresh is admin.
			r.Get("/podcasts", h.handlePodcastList)
			r.Get("/podcasts/episodes/newest", h.handlePodcastNewest)
			r.Get("/podcasts/episodes/{id}", h.handlePodcastEpisode)
			r.Get("/podcasts/episodes/{id}/stream", h.handlePodcastStream)
			r.Post("/podcasts/episodes/{id}/download", h.handlePodcastDownload)
			r.Get("/podcasts/{id}", h.handlePodcastGet)

			// Catalog browse over the shared library service.
			r.Get("/artists", h.handleListArtists)
			r.Get("/artists/{id}", h.handleGetArtist)
			r.Get("/albums", h.handleListAlbums)
			r.Get("/albums/{id}", h.handleGetAlbum)
			r.Get("/songs", h.handleSongsByGenre)
			r.Get("/songs/radio", h.handleSonicRadio)
			r.Post("/songs/autodj", h.handleAutoDJ)
			r.Get("/songs/{id}", h.handleGetSong)
			r.Get("/songs/{id}/sonic", h.handleSongSonic)
			r.Get("/songs/{id}/local", h.handleGetSongLocalStatus)
			r.Get("/songs/{id}/lyrics", h.handleGetSongLyrics)
			r.Get("/genres", h.handleGetGenres)
			r.Get("/search", h.handleSearch)

			// Favorites (star) and ratings on catalog items.
			r.Put("/songs/{id}/star", h.handleStarSong)
			r.Delete("/songs/{id}/star", h.handleUnstarSong)
			r.Put("/albums/{id}/star", h.handleStarAlbum)
			r.Delete("/albums/{id}/star", h.handleUnstarAlbum)
			r.Put("/artists/{id}/star", h.handleStarArtist)
			r.Delete("/artists/{id}/star", h.handleUnstarArtist)
			r.Put("/songs/{id}/rating", h.handleSetRating)
			r.Delete("/songs/{id}/rating", h.handleClearRating)
			r.Put("/albums/{id}/rating", h.handleSetRating)
			r.Delete("/albums/{id}/rating", h.handleClearRating)
			r.Put("/artists/{id}/rating", h.handleSetRating)
			r.Delete("/artists/{id}/rating", h.handleClearRating)

			// Scrobble a play (sets now-playing and, on submission, records it).
			r.Post("/scrobbles", h.handleScrobble)

			// Saved play queue (cross-device) and the now-playing feed.
			r.Get("/play-queue", h.handleGetPlayQueue)
			r.Put("/play-queue", h.handleSavePlayQueue)
			r.Get("/play-queue/events", h.handleStreamPlayQueue)
			r.Get("/play-queue/targets", h.handleListPlaybackTargets)
			r.Put("/play-queue/target", h.handleSetPlaybackTarget)
			r.Post("/play-queue/commands", h.handleSendPlayQueueCommand)
			r.Get("/now-playing", h.handleNowPlaying)

			// Mint short-lived signed stream/download URLs for the {id} track.
			r.Get("/songs/{id}/stream-url", h.handleStreamURL)

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

			// Playlist CRUD over the shared playlist service.
			r.Get("/playlists", h.handleListPlaylists)
			r.Post("/playlists", h.handleCreatePlaylist)
			r.Get("/playlists/{id}", h.handleGetPlaylist)
			r.Patch("/playlists/{id}", h.handleUpdatePlaylist)
			r.Delete("/playlists/{id}", h.handleDeletePlaylist)
			r.Put("/playlists/{id}/tracks", h.handleReplacePlaylistTracks)
			r.Post("/playlists/{id}/tracks/{position}/resolve", h.handleResolvePlaylistTrack)
			r.Post("/spotify-playlists/{spotifyId}/open", h.handleOpenSpotifyPlaylist)
			r.Put("/playlists/{id}/cover", h.handlePlaylistCover)
			r.Post("/playlists/{id}/cover/generate", h.handlePlaylistCoverGenerate)

			// Hall of Fame: the caller's personal top-tracks ranking (its own
			// dedicated tables, not a playlist — see core.HallOfFameService).
			r.Get("/hall-of-fame", h.handleGetHallOfFame)
			r.Put("/hall-of-fame/tracks", h.handleSetHallOfFameOrder)
			r.Post("/hall-of-fame/tracks", h.handleAddHallOfFameTrack)
			r.Patch("/hall-of-fame/tracks/{trackId}/note", h.handleSetHallOfFameNote)

			// Share links over the shared share service.
			r.Get("/shares", h.handleListShares)
			r.Post("/shares", h.handleCreateShare)
			r.Patch("/shares/{id}", h.handleUpdateShare)
			r.Delete("/shares/{id}", h.handleDeleteShare)

			// Collaborative / public playlists (extensions over the Subsonic API).
			r.Get("/playlists/public", h.handlePublicPlaylists)
			r.Put("/playlists/{id}/subscription", h.handleSubscribePlaylist)
			r.Delete("/playlists/{id}/subscription", h.handleUnsubscribePlaylist)
			r.Post("/playlists/{id}/collaborators", h.handleAddCollaborator)

			// Synchronized Jam sessions.
			r.Get("/jam/mine", h.handleJamMine)
			r.Get("/jam/invites/mine", h.handleJamInvitesMine)
			r.Delete("/jam/invites/{id}", h.handleJamInviteDismiss)
			r.Post("/jam", h.handleJamCreate)
			r.Get("/jam/{id}", h.handleJamState)
			r.Patch("/jam/{id}", h.handleJamUpdate)
			r.Get("/jam/{id}/events", h.handleJamEvents)
			r.Post("/jam/{id}/participants", h.handleJamJoin)
			r.Delete("/jam/{id}/participants/me", h.handleJamLeave)
			r.Post("/jam/{id}/invites", h.handleJamInvite)
			r.Delete("/jam/{id}", h.handleJamDelete)

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

			// Discovery + instance→instance subscriptions (proxied to the hub).

			// Admin API: every route below requires an admin caller.
			r.Route("/admin", func(r chi.Router) {
				r.Use(adminOnly)

				// User management.
				r.Get("/users", h.handleListUsers)
				r.Post("/users", h.handleCreateUser)
				r.Get("/users/{username}", h.handleGetUser)
				r.Patch("/users/{username}", h.handleUpdateUser)
				r.Delete("/users/{username}", h.handleDeleteUser)

				// Library scans.
				r.Get("/library/scan", h.handleScanStatus)
				r.Post("/library/scan", h.handleStartScan)

				// Internet radio feature toggle and station CRUD.
				r.Get("/radio", h.handleRadioAdmin)
				r.Put("/radio", h.handleRadioToggle)
				r.Post("/radio/stations", h.handleRadioCreate)
				r.Put("/radio/stations/{id}", h.handleRadioUpdate)
				r.Delete("/radio/stations/{id}", h.handleRadioDelete)

				// Podcast channel CRUD, refresh and directory providers.
				r.Post("/podcasts", h.handlePodcastCreate)
				r.Post("/podcasts/refresh", h.handlePodcastRefresh)
				r.Get("/podcasts/search", h.handlePodcastSearch)
				r.Get("/podcasts/providers", h.handlePodcastProviders)
				r.Put("/podcasts/providers/{name}", h.handlePodcastProviderUpdate)
				r.Delete("/podcasts/episodes/{id}", h.handlePodcastDeleteEpisode)
				r.Delete("/podcasts/{id}", h.handlePodcastDeleteChannel)

				// Library track management (list, edit, cover, delete).
				r.Get("/tracks", h.handleAdminTracks)
				r.Patch("/tracks/{id}", h.handleAdminTrackUpdate)
				r.Put("/tracks/{id}/cover", h.handleAdminTrackCover)
				r.Delete("/tracks/{id}", h.handleAdminTrackDelete)

				// Provider-download eviction sweep.
				r.Get("/cleanup", h.handleCleanup)
				r.Put("/cleanup", h.handleCleanupUpdate)
				r.Post("/cleanup/runs", h.handleCleanupRun)

				// Curated chart-playlist sync (force-run, otherwise weekly).
				r.Post("/charts/sync", h.handleChartsSync)
				r.Post("/autoplaylists/sync", h.handleAutoPlaylistsSync)
				r.Post("/concerts/sync", h.handleConcertsSync)

				// Local audio-analysis progress (toggled via /admin/settings).
				r.Get("/sonic", h.handleSonicStatus)
				r.Get("/sonic/duplicates", h.handleSonicDuplicates)

				// Runtime-configurable on-demand providers.
				r.Get("/providers", h.handleProviders)
				r.Post("/providers", h.handleProviderUpsert)
				r.Put("/providers/order", h.handleProviderReorder)
				r.Put("/providers/{name}/enabled", h.handleProviderEnable)
				r.Put("/providers/{name}/capabilities/{capability}", h.handleProviderCapability)
				r.Delete("/providers/{name}", h.handleProviderDelete)
				r.Get("/providers/{name}/logs", h.handleProviderLogs)

				// DB-backed runtime settings.
				r.Get("/settings", h.handleSettings)
				r.Patch("/settings", h.handleSettingsUpdate)

				// Live server log stream (SSE).
				r.Get("/logs/stream", h.handleStreamLogs)

				// Hub link lifecycle. GET refreshes the live name/sqid from the
				// hub; register bootstraps (links) under the configured user id; PATCH
				// pushes a name/sqid edit; DELETE unlinks (and deletes hub-side data).
				r.Get("/federation", h.handleFederationProfile)
				r.Post("/federation/register", h.handleFederationRegister)
				r.Patch("/federation", h.handleFederationUpdate)
				r.Delete("/federation", h.handleFederationUnlink)
				// Discovery + instance→instance subscriptions (proxied to the hub).
				r.Get("/federation/instances", h.handleFederationSearch)
				r.Get("/federation/subscriptions", h.handleFederationSubscriptions)
				r.Post("/federation/subscriptions", h.handleFederationSubscribe)
				r.Delete("/federation/subscriptions/{id}", h.handleFederationUnsubscribe)

				// Smart-playlists feature toggle.
				r.Get("/smart-playlists", h.handleSmartPlaylistsAdmin)
				r.Put("/smart-playlists", h.handleSmartPlaylistsToggle)

				// Wrapped feature toggle.
				r.Get("/wrapped", h.handleWrappedAdmin)
				r.Put("/wrapped", h.handleWrappedUpdate)

				// Offline-downloads feature toggle.
				r.Get("/offline", h.handleOfflineAdmin)
				r.Put("/offline", h.handleOfflineUpdate)

				// Hall of Fame feature toggle.
				r.Get("/hall-of-fame", h.handleHallOfFameAdmin)
				r.Put("/hall-of-fame", h.handleHallOfFameToggle)

				// Concert-discovery settings (enable + API keys).
				r.Get("/concerts", h.handleConcertsAdmin)
				r.Put("/concerts", h.handleConcertsUpdate)

				// Last.fm scrobbling settings (enable + API key/secret).
				r.Get("/lastfm", h.handleLastFmAdmin)
				r.Put("/lastfm", h.handleLastFmAdminUpdate)
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
