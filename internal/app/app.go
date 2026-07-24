// Package app wires together configuration, persistence, services and the HTTP
// API into a runnable application.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/api/docs"
	"github.com/immerle/immerle/internal/api/immerle"
	"github.com/immerle/immerle/internal/api/subsonic"
	"github.com/immerle/immerle/internal/autoplaylists"
	"github.com/immerle/immerle/internal/bandcamp"
	"github.com/immerle/immerle/internal/charts"
	"github.com/immerle/immerle/internal/concerts"
	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/federation"
	"github.com/immerle/immerle/internal/importer"
	"github.com/immerle/immerle/internal/logging"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/outbox"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/providers"
	"github.com/immerle/immerle/internal/providers/httpprovider"
	"github.com/immerle/immerle/internal/reccobeats"
	"github.com/immerle/immerle/internal/scanner"
	"github.com/immerle/immerle/internal/server"
	"github.com/immerle/immerle/internal/stream"
	webui "github.com/immerle/immerle/ui"
)

// App holds the assembled application.
type App struct {
	cfg           config.Config
	logger        *slog.Logger
	database      *db.DB
	store         *persistence.Store
	scanner       *scanner.Scanner
	watcher       *scanner.Watcher
	onDemand      *core.CatalogService
	federation    *federation.Service
	outbox        *outbox.Worker
	enricher      *core.ArtistImageEnricher
	evictor       *core.Evictor
	charts        *charts.Service
	autoplaylists *autoplaylists.Service
	concerts      *concerts.Service
	purchases     *core.PurchasesService
	logPruner     *core.LogPruner
	settings      *core.SettingsService
	imports       *importer.Service
	handler       http.Handler

	scanPaths []string
	// watch is captured at boot; changing it requires a restart.
	watch bool
	// wg tracks background workers so Run can wait for them to drain before
	// returning (and the caller closing the DB), avoiding "database is closed".
	wg sync.WaitGroup
}

// builtinProviderDefs declares the compiled-in providers managed via the admin
// API. Their credentials live in the config JSON (no env vars). Only those with
// a registered factory are surfaced.
func builtinProviderDefs() []core.BuiltinDef {
	all := []core.BuiltinDef{
		// CC-licensed; no credentials. First in order (highest search priority) and
		// enabled by default.
		{Name: "free-music-archive", DefaultConfig: `{}`, DefaultEnabled: true},
		// Public-domain / CC; no credentials → enabled by default.
		{Name: "internet-archive", DefaultConfig: `{"params":{"max_items":"8"}}`, DefaultEnabled: true},
		// Needs a free API key → seeded disabled with a token placeholder to fill
		// in via the admin UI before enabling.
		{Name: "jamendo", DefaultConfig: `{"params":{"client_id":"<JAMENDO_TOKEN>","audioformat":"mp32"}}`, DefaultEnabled: false},
	}
	out := make([]core.BuiltinDef, 0, len(all))
	for _, d := range all {
		if providers.HasFactory(d.Name) {
			out = append(out, d)
		}
	}
	return out
}

// catalogResolver adapts the on-demand CatalogService to importer.ContentResolver:
// search the content providers, then turn a chosen remote track into a local one.
type catalogResolver struct{ svc *core.CatalogService }

func (c catalogResolver) SearchTracks(ctx context.Context, query string, limit int) ([]models.Track, error) {
	return c.svc.RemoteSearch(ctx, query, limit)
}

func (c catalogResolver) Resolve(ctx context.Context, userID, trackID string) (string, error) {
	t, _, _, err := c.svc.Resolve(ctx, userID, trackID)
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

// hubPlaylistFetcher adapts the federation service to importer.HubFetcher: it
// fetches an external (e.g. Spotify) playlist through the hub and converts it to
// the importer's playlist shape.
type hubPlaylistFetcher struct{ fed *federation.Service }

func (h hubPlaylistFetcher) Available() bool { return h.fed != nil && h.fed.HubConfigured() }

func (h hubPlaylistFetcher) FetchPlaylist(ctx context.Context, source, ref string) (importer.Playlist, error) {
	ep, err := h.fed.FetchExternalPlaylist(ctx, source, ref)
	if err != nil {
		return importer.Playlist{}, err
	}
	pl := importer.Playlist{Name: ep.Name}
	for _, t := range ep.Tracks {
		pl.Tracks = append(pl.Tracks, importer.Track{Title: t.Title, Artist: t.Artist, Album: t.Album})
	}
	return pl, nil
}

// transcodeConfig maps the runtime transcode settings to the streamer's config
// type, deriving the cache dir under the data dir.
func transcodeConfig(t models.TranscodeRuntime, dataDir string) config.TranscodeConfig {
	profiles := make([]config.TranscodeProfile, 0, len(t.Profiles))
	for _, p := range t.Profiles {
		profiles = append(profiles, config.TranscodeProfile{
			Name: p.Name, Format: p.Format, BitRate: p.BitRate, FFmpegArgs: p.FFmpegArgs,
		})
	}
	return config.TranscodeConfig{
		FFmpegPath:  t.FFmpegPath,
		FFprobePath: t.FFprobePath,
		CacheDir:    filepath.Join(dataDir, "transcode"),
		Profiles:    profiles,
	}
}

// federationConfig maps the runtime federation settings to the federation
// service's config type. hubURL is the resolved hub endpoint (hardcoded, or
// the DEV_IMMERLE_HUB_URL override from the bootstrap config).
func federationConfig(f models.FederationRuntime, hubURL string) config.FederationConfig {
	return config.FederationConfig{
		HubURL:          hubURL,
		UserID:          f.UserID,
		InstanceID:      f.InstanceID,
		Sqid:            f.Sqid,
		InstanceName:    f.InstanceName,
		PrivateKey:      f.PrivateKey,
		SyncPlaylists:   f.SyncPlaylists,
		ExportScrobbles: f.ExportScrobbles,
	}
}

// hubHost extracts the hostname from the configured hub URL (for the cover
// service's SSRF allowlist); a malformed URL yields "" (matches nothing).
func hubHost(hubURL string) string {
	u, err := url.Parse(hubURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// New builds the application from configuration.
func New(cfg config.Config) (*App, error) {
	logger, logHub := logging.New(cfg.Log.Level)
	// Make the configured logger the process default so package-level helpers
	// (e.g. the API's writeInternal) log through it instead of the stderr default.
	slog.SetDefault(logger)

	database, err := db.Open(cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		return nil, err
	}

	// Migrations get their own budget so a slow migration can't starve the later
	// bootstrap steps of their share of the timeout.
	migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = database.Migrate(migrateCtx)
	migrateCancel()
	if err != nil {
		return nil, err
	}
	logger.Info("migrations applied")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := persistence.New(database)

	dataDir := cfg.Library.DataDir
	coversDir := filepath.Join(dataDir, "covers")
	downloadDir := filepath.Join(dataDir, "library")
	podcastsDir := filepath.Join(dataDir, "podcasts")

	// Runtime settings + auth secret live in data/configuration.yaml (admin-
	// managed; the secret is auto-generated there, migrating the legacy
	// data/secret file if present). AUTH_SECRET, if set, overrides the file.
	settingsSvc, err := core.NewSettingsService(
		filepath.Join(dataDir, "configuration.yaml"), cfg.Auth.Secret, filepath.Join(dataDir, "secret"), logger)
	if err != nil {
		return nil, err
	}
	rs := settingsSvc.Get()

	authSvc, err := core.NewAuthService(store.Users, store.APITokens, store.Devices, settingsSvc.Secret())
	if err != nil {
		return nil, err
	}
	authSvc.WithLDAP(settingsSvc)

	// First-run setup: the initial admin is created either via the setup API, or
	// here from ADMIN_USERNAME/ADMIN_PASSWORD when set.
	setupSvc, err := core.NewSetupService(store.Users, authSvc, cfg.Auth.RequireSetupToken)
	if err != nil {
		return nil, err
	}

	if cfg.Auth.AdminUsername != "" {
		switch _, err := setupSvc.BootstrapFromEnv(ctx, strings.TrimSpace(cfg.Auth.AdminUsername), cfg.Auth.AdminPassword); {
		case err == nil:
			logger.Info("admin account bootstrapped from ADMIN_USERNAME/ADMIN_PASSWORD")
		case errors.Is(err, core.ErrAlreadyInitialized):
			// Already set up (earlier run, or via the setup UI) — leave it alone.
		default:
			return nil, fmt.Errorf("admin bootstrap from ADMIN_USERNAME/ADMIN_PASSWORD: %w", err)
		}
	}

	// Announce setup mode when the server has no admin yet.
	if initialized, err := setupSvc.IsInitialized(ctx); err == nil && !initialized {
		if setupSvc.TokenRequired() {
			if err := setupSvc.PersistToken(dataDir); err != nil {
				logger.Warn("could not persist setup token", "error", err)
			}
			logger.Warn("first-run setup required — create the admin via POST /setup/init",
				"setupToken", setupSvc.Token(), "tokenFile", filepath.Join(dataDir, "setup-token"))
		} else {
			logger.Warn("first-run setup required — create the admin via POST /setup/init (no token required)")
		}
	}

	// Transcoding config comes from the runtime settings (restart-required).
	transcodeCfg := transcodeConfig(rs.Transcode, dataDir)
	extractor := scanner.NewExtractor(transcodeCfg.FFprobePath)
	scan := scanner.New(store.Catalog, store.Genres, extractor, coversDir, logger)

	// Federated playlist covers are fetched from the hub, so its host needs the
	// cover service's SSRF allowlist alongside the built-in provider hosts.
	coverSvc := stream.NewCoverService(store.Catalog, coversDir, hubHost(cfg.HubURL))
	streamer := stream.NewStreamer(transcodeCfg, logger)
	nowPlaying := core.NewNowPlayingTracker(10 * time.Minute)
	activitySvc := core.NewActivityService(store.Activity)
	jamSvc := core.NewJamService(store.Jam)
	podcastSvc := core.NewPodcastService(store.Podcasts, podcastsDir, logger)

	scanPaths := append([]string{}, cfg.Library.Paths...)

	// On-demand catalog (S5): always running, idles with no provider enabled.
	// Provider config changes are applied to this live registry and the DB
	// together by the manager — hot, no restart.
	registry := core.NewProviderRegistry()
	// Both kinds are configured via the admin API (JSON config). A built-in is a
	// compiled-in factory whose credentials come from its config JSON; a dynamic
	// provider is a content-neutral HTTP service.
	build := func(c models.ProviderConfig) (providers.Provider, error) {
		if c.Builtin() {
			cfg, err := providers.ParseConfig(c.Config)
			if err != nil {
				return nil, err
			}
			return providers.Build(c.Name, cfg)
		}
		return httpprovider.New(c.Name, c.Endpoint, c.Config)
	}
	providerMgr := core.NewProviderManager(store.ProviderConfigs, registry, build, builtinProviderDefs(), logger)
	if err := providerMgr.Load(ctx); err != nil {
		logger.Warn("loading providers failed", "error", err)
	}

	onDemand := core.NewCatalogService(core.CatalogServiceConfig{
		Catalog:      store.Catalog,
		Downloads:    store.Downloads,
		Registry:     registry,
		Scanner:      scan,
		Settings:     settingsSvc, // hot-reloadable: default/auto-download/timeout
		DownloadDir:  downloadDir,
		FFmpegPath:   transcodeCfg.FFmpegPath,
		Logger:       logger,
		ProviderLogs: store.ProviderLogs,
	})
	// Downloaded tracks live under downloadDir; scan it too.
	scanPaths = append(scanPaths, downloadDir)

	// Artist avatars come from the on-demand provider; always on, idles if no
	// provider exposes the artist-image capability.
	enricher := core.NewArtistImageEnricher(store.Catalog, core.NewProviderImageLookup(onDemand), coversDir, time.Second, logger)

	// Library analytics (counts + total size/duration), cached and recomputed at
	// each scan so the analytics endpoint never SUMs over every track on request.
	libraryStats := core.NewLibraryStatsService(store.Catalog, logger)
	// After every scan: refresh the cached stats and wake the avatar enricher.
	scan.SetOnComplete(func(ctx context.Context, _ scanner.Result) {
		if _, err := libraryStats.Refresh(ctx); err != nil {
			logger.Warn("library stats refresh failed", "error", err)
		}
		enricher.Wake()
	})

	// Federation client (S7): always built, config is read live (hot-reloadable,
	// no restart), Run() idles while disabled. Playlist owner is resolved lazily
	// so enabling federation later still works.
	var fedResolver federation.Resolver
	if onDemand != nil {
		fedResolver = onDemand
	}
	fed := federation.New(
		func() config.FederationConfig { return federationConfig(settingsSvc.Get().Federation, cfg.HubURL) },
		store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, fedResolver, logger)
	fed.SetOwnerResolver(func(ctx context.Context) (string, error) { return firstAdmin(ctx, store.Users) })
	// Persist hub-issued identity (instance UUID, sqid, private key, name) back
	// into the runtime settings after bootstrap/update. Empty fields are left
	// unchanged so an update can touch only name/sqid.
	fed.SetCredentialsSaver(func(_ context.Context, creds federation.Credentials) error {
		next := settingsSvc.Get()
		if creds.InstanceID != "" {
			next.Federation.InstanceID = creds.InstanceID
		}
		if creds.Sqid != "" {
			next.Federation.Sqid = creds.Sqid
		}
		if creds.PrivateKey != "" {
			next.Federation.PrivateKey = creds.PrivateKey
		}
		if creds.Name != "" {
			next.Federation.InstanceName = creds.Name
		}
		_, _, err := settingsSvc.Update(next)
		return err
	})
	// Clear hub identity on unlink (resets the instance to the unlinked state).
	fed.SetCredentialsClearer(func(_ context.Context) error {
		next := settingsSvc.Get()
		next.Federation.UserID = ""
		next.Federation.InstanceID = ""
		next.Federation.Sqid = ""
		next.Federation.InstanceName = ""
		next.Federation.PrivateKey = ""
		_, _, err := settingsSvc.Update(next)
		return err
	})

	// Generic durable async queue. Subsystems register handlers per job "kind";
	// a single worker drains it with retry/backoff. Reusable beyond federation.
	outboxWorker := outbox.NewWorker(store.Outbox, logger)
	// Outbound playlist sync registers itself on the outbox: it pushes public
	// playlists to the hub (upsert) or removes them (delete), with content-
	// addressed cover de-dup. PlaylistService enqueues on every mutation.
	playlistSyncer := federation.NewPlaylistSyncer(fed, outboxWorker, store.PlaylistSync, store.CoverUploads, store.Playlists, coverSvc, logger)

	// Playlist import (e.g. Spotify): the source playlist is fetched through the
	// hub (which holds the third-party credentials), then each track is resolved
	// against the on-demand content providers and downloaded into a new playlist.
	importSvc := importer.NewService(store.Imports, store.Playlists,
		catalogResolver{onDemand}, hubPlaylistFetcher{fed}, settingsSvc.ImportSources, logger)

	// Cleanup of unused provider downloads. Enabled state + retention window are
	// read live from the runtime settings (hot); the cadence is read at boot.
	evictor := core.NewEvictor(store.Catalog, store.Downloads,
		settingsSvc.CleanupEnabled, settingsSvc.CleanupMaxAge, settingsSvc.CleanupInterval(), logger)

	// Curated chart playlists (global + a handful of major markets), synced
	// weekly from kworb-net-api. Materializes as public, federated-style
	// playlists (same mechanism as hub imports), owned by the first admin.
	chartsSvc := charts.New(store.Playlists, "", nil, logger)
	chartsSvc.SetOwnerResolver(func(ctx context.Context) (string, error) { return firstAdmin(ctx, store.Users) })

	// Genre/decade playlists ("Rock", "Rap", "1990s"...) and personal listening
	// lists ("Top du mois", "On Repeat", "Favoris oubliés"), auto-generated and
	// refreshed daily as real playlists. Same materializer mechanism and owner
	// resolution (for the shared genre/decade ones) as chartsSvc above.
	autoplaylistsSvc := autoplaylists.New(store.Catalog, store.Genres, store.Wrapped, store.Annotations, store.Users, store.Playlists, logger)
	autoplaylistsSvc.SetOwnerResolver(func(ctx context.Context) (string, error) { return firstAdmin(ctx, store.Users) })
	// "Découvertes": a per-user recommendation mix from the keyless ReccoBeats
	// API (https://reccobeats.com), seeded from each user's top tracks and
	// synced daily along with the other personal lists above.
	autoplaylistsSvc.SetRecommender(reccobeats.NewClient())

	// Concert discovery: matches each user-with-a-city's top-listened artists
	// against Ticketmaster/Skiddle, once daily. Disabled by default (needs at
	// least one API key, set from the admin settings).
	concertsSvc := concerts.New(store.Users, store.Wrapped, store.Concerts, settingsSvc.ConcertsConfig, logger)

	// Bandcamp purchase import: a user pastes their personal session cookie
	// (no official OAuth exists) and we download+ingest their purchased
	// albums/tracks, same uploads tree a manual upload lands in.
	purchasesSvc, err := core.NewPurchasesService(store.BandcampConns, store.BandcampImports, bandcamp.NewClient(),
		store.Catalog, scan, filepath.Join(downloadDir, "uploads"), settingsSvc.Secret(), logger)
	if err != nil {
		return nil, fmt.Errorf("purchases service: %w", err)
	}

	// Daily retention sweep over persisted diagnostic logs. The window is read
	// live from the runtime settings; register any future log table here.
	logPruner := core.NewLogPruner(settingsSvc.LogRetention, 24*time.Hour, logger, store.ProviderLogs)

	subHandler := subsonic.NewHandler(subsonic.Deps{
		Auth:             authSvc,
		Catalog:          store.Catalog,
		Genres:           store.Genres,
		Annotations:      store.Annotations,
		Playlists:        store.Playlists,
		PlaylistSync:     playlistSyncer,
		PlayQueues:       store.PlayQueues,
		Scrobbles:        store.Scrobbles,
		Shares:           store.Shares,
		Users:            store.Users,
		Radio:            store.Radio,
		Podcasts:         podcastSvc,
		Settings:         settingsSvc,
		Cover:            coverSvc,
		Streamer:         streamer,
		NowPlaying:       nowPlaying,
		Scanner:          scan,
		OnDemand:         onDemand,
		Activity:         activitySvc,
		MusicFolderPaths: cfg.Library.Paths,
		BaseURL:          baseURL(cfg),
		Logger:           logger,
	})

	gosHandler := immerle.NewHandler(immerle.Deps{
		Auth:           authSvc,
		Users:          store.Users,
		Activity:       activitySvc,
		Playlists:      store.Playlists,
		PlaylistSync:   playlistSyncer,
		Jam:            jamSvc,
		Setup:          setupSvc,
		Federation:     fed,
		Cleanup:        evictor,
		Charts:         chartsSvc,
		AutoPlaylists:  autoplaylistsSvc,
		Concerts:       store.Concerts,
		ConcertsSync:   concertsSvc,
		Purchases:      purchasesSvc,
		Providers:      providerMgr,
		Settings:       settingsSvc,
		SmartPlaylists: store.SmartPlaylists,
		Radio:          store.Radio,
		Podcasts:       podcastSvc,
		Wrapped:        store.Wrapped,
		HallOfFame:     store.HallOfFame,
		Catalog:        store.Catalog,
		Annotations:    store.Annotations,
		Genres:         store.Genres,
		Scrobbles:      store.Scrobbles,
		PlayQueues:     store.PlayQueues,
		NowPlaying:     nowPlaying,
		OnDemand:       onDemand,
		Streamer:       streamer,
		Cover:          coverSvc,
		Shares:         store.Shares,
		BaseURL:        baseURL(cfg),
		SigningKey:     settingsSvc.Secret(),
		LibraryStats:   libraryStats,
		Imports:        importSvc,
		Scanner:        scan,
		UploadsDir:     filepath.Join(downloadDir, "uploads"),
		CoversDir:      coversDir,
		Logger:         logger,
		LogHub:         logHub,
	})

	// Warm the analytics cache from whatever is already indexed (the post-scan
	// hook keeps it fresh thereafter).
	if _, err := libraryStats.Refresh(ctx); err != nil {
		logger.Warn("initial library stats failed", "error", err)
	}

	// Seed the built-in internet radio stations (idempotent).
	if err := store.Radio.EnsureBuiltins(ctx); err != nil {
		logger.Warn("seeding built-in radio stations failed", "error", err)
	}

	// Enable the default-on podcast directory providers on first boot (idempotent).
	if err := podcastSvc.EnsureDefaults(ctx); err != nil {
		logger.Warn("seeding default podcast providers failed", "error", err)
	}

	mux := chi.NewRouter()
	mux.HandleFunc("/ping", healthHandler)
	mux.HandleFunc("/share/*", shareHandler(store.Shares))
	subHandler.Register(mux)
	gosHandler.Register(mux)
	docs.Register(mux) // /openapi.json, /openapi.yaml, /swagger/
	// Embedded web app: every API route wins; unmatched falls through here
	// (404 until the UI is built into the binary).
	mux.NotFound(webui.Handler().ServeHTTP)

	return &App{
		cfg:           cfg,
		logger:        logger,
		database:      database,
		store:         store,
		scanner:       scan,
		watcher:       scanner.NewWatcher(scan, scanPaths, settingsSvc.ScanInterval, logger),
		onDemand:      onDemand,
		federation:    fed,
		outbox:        outboxWorker,
		enricher:      enricher,
		evictor:       evictor,
		charts:        chartsSvc,
		autoplaylists: autoplaylistsSvc,
		concerts:      concertsSvc,
		purchases:     purchasesSvc,
		logPruner:     logPruner,
		settings:      settingsSvc,
		imports:       importSvc,
		// Panic recovery outermost (so it catches every downstream handler), then
		// security headers (apply to every response), then CORS (answers preflight
		// before routing), then logging. Origins are read live from the runtime
		// settings (hot-reloadable).
		handler:   recoverMiddleware(logger, securityHeadersMiddleware(corsMiddleware(settingsSvc.CORSOrigins, loggingMiddleware(logger, mux)))),
		scanPaths: scanPaths,
		watch:     rs.Scan.Watch,
	}, nil
}

// Run starts background workers and the HTTP server, blocking until ctx is done.
// On shutdown it cancels the workers and waits for them to drain before
// returning, so a subsequent Close() never shuts the DB out from under a worker.
func (a *App) Run(ctx context.Context) error {
	// Own cancellable scope so workers are stopped even if the server returns for
	// a reason other than ctx cancellation (e.g. a bind failure).
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if len(a.scanPaths) > 0 {
		a.spawn(func() {
			// Use the run ctx so a SIGTERM during the initial scan cancels it.
			if _, err := a.scanner.ScanPaths(ctx, a.scanPaths); err != nil {
				a.logger.Warn("initial scan failed", "error", err)
			}
			// Newly scanned artists need avatars now, not at the next idle tick.
			if a.enricher != nil {
				a.enricher.Wake()
			}
		})
	}
	if a.watch && len(a.scanPaths) > 0 {
		a.spawn(func() {
			if err := a.watcher.Run(ctx); err != nil {
				a.logger.Warn("watcher stopped", "error", err)
			}
		})
	}
	if a.onDemand != nil {
		a.spawn(func() { a.onDemand.Worker(ctx) })
	}
	if a.federation != nil {
		a.spawn(func() { a.federation.Run(ctx) })
		a.spawn(func() { a.federation.RunStream(ctx) })
	}
	if a.outbox != nil {
		a.spawn(func() { a.outbox.Run(ctx) })
	}
	if a.enricher != nil {
		// Short idle so incrementally-added artists are picked up promptly; the
		// post-scan Wake() handles the cold-start case immediately.
		a.spawn(func() { a.enricher.Run(ctx, 2*time.Minute) })
	}
	if a.evictor != nil {
		// Always started; it self-gates on the runtime enabled flag.
		a.spawn(func() { a.evictor.Run(ctx) })
	}
	if a.charts != nil {
		a.spawn(func() { a.charts.Run(ctx) })
	}
	if a.autoplaylists != nil {
		a.spawn(func() { a.autoplaylists.Run(ctx) })
	}
	if a.concerts != nil {
		// Always started; SyncNow self-gates on the runtime enabled flag, same
		// as the evictor.
		a.spawn(func() { a.concerts.Run(ctx) })
	}
	if a.purchases != nil {
		a.spawn(func() { a.purchases.Worker(ctx) })
	}
	if a.logPruner != nil {
		a.spawn(func() { a.logPruner.Run(ctx) })
	}
	if a.imports != nil {
		a.spawn(func() { a.imports.Worker(ctx) })
	}

	srv := server.New(a.cfg.Server.Address, a.handler, a.logger)
	err := srv.Run(ctx)
	// Stop workers and wait for them to finish before returning, so Close() can
	// safely shut the DB.
	cancel()
	a.wg.Wait()
	return err
}

// spawn runs fn as a tracked background worker so Run can wait for it on
// shutdown.
func (a *App) spawn(fn func()) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		fn()
	}()
}

// Close releases resources.
func (a *App) Close() error {
	// Best-effort: persist any last-seen/last-used touches buffered in memory
	// before closing the DB, so a graceful shutdown doesn't lose them (see
	// DeviceRepo.TouchSeen / APITokenRepo.TouchLastUsed).
	ctx := context.Background()
	if err := a.store.Devices.FlushSeen(ctx); err != nil {
		a.logger.Warn("flushing device last-seen on shutdown failed", "error", err)
	}
	if err := a.store.APITokens.FlushLastUsed(ctx); err != nil {
		a.logger.Warn("flushing token last-used on shutdown failed", "error", err)
	}
	return a.database.Close()
}

func firstAdmin(ctx context.Context, users *persistence.UserRepo) (string, error) {
	list, err := users.List(ctx)
	if err != nil {
		return "", err
	}
	for _, u := range list {
		if u.IsAdmin {
			return u.ID, nil
		}
	}
	if len(list) > 0 {
		return list[0].ID, nil
	}
	return "", fmt.Errorf("no users")
}

func baseURL(cfg config.Config) string {
	return "http://localhost" + cfg.Server.Address
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
