// Package app wires together configuration, persistence, services and the HTTP
// API into a runnable application.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gossignol/gossignol/internal/api/docs"
	"github.com/gossignol/gossignol/internal/api/gossignol"
	"github.com/gossignol/gossignol/internal/api/subsonic"
	"github.com/gossignol/gossignol/internal/config"
	"github.com/gossignol/gossignol/internal/core"
	"github.com/gossignol/gossignol/internal/db"
	"github.com/gossignol/gossignol/internal/federation"
	"github.com/gossignol/gossignol/internal/importer"
	"github.com/gossignol/gossignol/internal/logging"
	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/persistence"
	"github.com/gossignol/gossignol/internal/providers"
	"github.com/gossignol/gossignol/internal/providers/httpprovider"
	"github.com/gossignol/gossignol/internal/scanner"
	"github.com/gossignol/gossignol/internal/server"
	"github.com/gossignol/gossignol/internal/stream"
)

// App holds the assembled application.
type App struct {
	cfg        config.Config
	logger     *slog.Logger
	database   *db.DB
	store      *persistence.Store
	scanner    *scanner.Scanner
	watcher    *scanner.Watcher
	onDemand   *core.CatalogService
	federation *federation.Service
	enricher   *core.ArtistImageEnricher
	evictor    *core.Evictor
	settings   *core.SettingsService
	imports    *importer.Service
	handler    http.Handler

	scanPaths []string
	// watch is the runtime "watch" setting captured at boot (changing it needs a
	// restart, so the running process uses the boot value).
	watch bool
}

// builtinProviderDefs declares the compiled-in providers managed via the admin
// API. Their credentials live in the config JSON (no env vars). Only those with
// a registered factory are surfaced.
func builtinProviderDefs() []core.BuiltinDef {
	all := []core.BuiltinDef{
		// Public-domain / CC; no credentials → enabled by default.
		{Name: "internet-archive", DefaultConfig: `{"max_items":"8"}`, DefaultEnabled: true},
		// Needs a free API key → seeded disabled with a token placeholder to fill
		// in via the admin UI before enabling.
		{Name: "jamendo", DefaultConfig: `{"client_id":"<JAMENDO_TOKEN>","audioformat":"mp32"}`, DefaultEnabled: false},
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
	pl := importer.Playlist{Name: ep.Name, Description: ep.Description}
	for _, t := range ep.Tracks {
		pl.Tracks = append(pl.Tracks, importer.Track{
			Title: t.Title, Artist: t.Artist, Album: t.Album, ISRC: t.ISRC, Duration: t.Duration,
		})
	}
	return pl, nil
}

// jsonToSettings flattens a provider config JSON object into the string settings
// map consumed by built-in provider factories.
func jsonToSettings(configJSON string) map[string]string {
	out := map[string]string{}
	var raw map[string]any
	if err := json.Unmarshal([]byte(configJSON), &raw); err != nil {
		return out
	}
	for k, v := range raw {
		out[k] = fmt.Sprint(v)
	}
	return out
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
// service's config type.
func federationConfig(f models.FederationRuntime) config.FederationConfig {
	return config.FederationConfig{
		Enabled:         f.Enabled,
		HubURL:          f.HubURL,
		PublicKey:       f.PublicKey,
		PrivateKey:      f.PrivateKey,
		SyncInterval:    time.Duration(f.SyncIntervalSeconds) * time.Second,
		ResolveMissing:  f.ResolveMissing,
		ExportScrobbles: f.ExportScrobbles,
	}
}

// New builds the application from configuration.
func New(cfg config.Config) (*App, error) {
	logger := logging.New(cfg.Log.Level, cfg.Log.Format)

	database, err := db.Open(cfg.Database.Driver, cfg.Database.DSN,
		cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := database.Migrate(ctx); err != nil {
		return nil, err
	}
	logger.Info("migrations applied")

	store := persistence.New(database)

	// Derived data directories.
	dataDir := cfg.Library.DataDir
	coversDir := filepath.Join(dataDir, "covers")
	downloadDir := filepath.Join(dataDir, "library")

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

	// First-run setup. The initial admin can only be created via the setup API.
	setupSvc, err := core.NewSetupService(store.Users, authSvc, cfg.Auth.RequireSetupToken)
	if err != nil {
		return nil, err
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

	coverSvc := stream.NewCoverService(store.Catalog, coversDir)
	streamer := stream.NewStreamer(transcodeCfg, logger)
	nowPlaying := core.NewNowPlayingTracker(10 * time.Minute)
	activitySvc := core.NewActivityService(store.Activity, store.Friends, store.Users)
	jamSvc := core.NewJamService(store.Jam)

	scanPaths := append([]string{}, cfg.Library.Paths...)

	// On-demand catalog (S5). It is always running; with no enabled provider it
	// simply has nothing to search/download (equivalent to "off"). Provider config
	// changes (add/edit/enable/reorder) are applied to this live registry and the
	// DB together by the manager — hot, no restart.
	registry := core.NewProviderRegistry()
	// Both kinds are configured via the admin API (JSON config). A built-in is a
	// compiled-in factory whose credentials come from its config JSON; a dynamic
	// provider is a content-neutral HTTP service.
	build := func(c models.ProviderConfig) (providers.Provider, error) {
		if c.Builtin() {
			return providers.Build(c.Name, jsonToSettings(c.Config))
		}
		return httpprovider.New(c.Name, c.Endpoint, c.Config)
	}
	providerMgr := core.NewProviderManager(store.ProviderConfigs, registry, build, builtinProviderDefs(), logger)
	if err := providerMgr.Load(ctx); err != nil {
		logger.Warn("loading providers failed", "error", err)
	}

	onDemand := core.NewCatalogService(core.CatalogServiceConfig{
		Catalog:     store.Catalog,
		Downloads:   store.Downloads,
		Registry:    registry,
		Scanner:     scan,
		Settings:    settingsSvc, // hot-reloadable: default/auto-download/timeout
		DownloadDir: downloadDir,
		FFmpegPath:  transcodeCfg.FFmpegPath,
		Logger:      logger,
	})
	// Downloaded tracks live under downloadDir; scan it too.
	scanPaths = append(scanPaths, downloadDir)

	// Artist avatars come from the on-demand provider (where artists come from).
	// Always on: if a provider exposes the artist-image capability, avatars are
	// fetched through it; otherwise this finds none and idles.
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

	// Federation client (S7). Always built and reads its config live — enabling/
	// disabling, the hub URL/keys, the sync interval and the feature flags are all
	// hot-reloadable (no restart). Run() idles while disabled. The owner of
	// federated playlists is resolved lazily so enabling it later still works.
	var fedResolver federation.Resolver
	if onDemand != nil {
		fedResolver = onDemand
	}
	fed := federation.New(
		func() config.FederationConfig { return federationConfig(settingsSvc.Get().Federation) },
		store.Catalog, store.Playlists, store.Scrobbles, fedResolver, logger)
	fed.SetOwnerResolver(func(ctx context.Context) (string, error) { return firstAdmin(ctx, store.Users) })

	// Playlist import (e.g. Spotify): the source playlist is fetched through the
	// hub (which holds the third-party credentials), then each track is resolved
	// against the on-demand content providers and downloaded into a new playlist.
	importSvc := importer.NewService(store.Imports, store.Playlists,
		catalogResolver{onDemand}, hubPlaylistFetcher{fed}, settingsSvc.ImportSources, logger)

	// Cleanup of unused provider downloads. Enabled state + retention window are
	// read live from the runtime settings (hot); the cadence is read at boot.
	evictor := core.NewEvictor(store.Catalog, store.Downloads,
		settingsSvc.CleanupEnabled, settingsSvc.CleanupMaxAge, settingsSvc.CleanupInterval(), logger)

	subHandler := subsonic.NewHandler(subsonic.Deps{
		Auth:             authSvc,
		Catalog:          store.Catalog,
		Genres:           store.Genres,
		Annotations:      store.Annotations,
		Playlists:        store.Playlists,
		PlayQueues:       store.PlayQueues,
		Scrobbles:        store.Scrobbles,
		Shares:           store.Shares,
		Users:            store.Users,
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

	gosHandler := gossignol.NewHandler(gossignol.Deps{
		Auth:         authSvc,
		Users:        store.Users,
		Friends:      store.Friends,
		Activity:     activitySvc,
		Playlists:    store.Playlists,
		Jam:          jamSvc,
		Setup:        setupSvc,
		Federation:   fed,
		Cleanup:      evictor,
		Providers:    providerControllerOrNil(providerMgr),
		Settings:     settingsSvc,
		Catalog:      store.Catalog,
		OnDemand:     onDemand,
		LibraryStats: libraryStats,
		Imports:      importSvc,
		Logger:       logger,
	})

	// Warm the analytics cache from whatever is already indexed (the post-scan
	// hook keeps it fresh thereafter).
	if _, err := libraryStats.Refresh(ctx); err != nil {
		logger.Warn("initial library stats failed", "error", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", healthHandler)
	mux.HandleFunc("/share/", shareHandler(store.Shares, logger))
	subHandler.Register(mux)
	gosHandler.Register(mux)
	docs.Register(mux) // /openapi.json, /openapi.yaml, /swagger/

	return &App{
		cfg:        cfg,
		logger:     logger,
		database:   database,
		store:      store,
		scanner:    scan,
		watcher:    scanner.NewWatcher(scan, scanPaths, settingsSvc.ScanInterval, logger),
		onDemand:   onDemand,
		federation: fed,
		enricher:   enricher,
		evictor:    evictor,
		settings:   settingsSvc,
		imports:    importSvc,
		// CORS is outermost so preflight requests are answered before routing.
		// Origins are read live from the runtime settings (hot-reloadable).
		handler:   corsMiddleware(settingsSvc.CORSOrigins, loggingMiddleware(logger, mux)),
		scanPaths: scanPaths,
		watch:     rs.Scan.Watch,
	}, nil
}

// Run starts background workers and the HTTP server, blocking until ctx is done.
func (a *App) Run(ctx context.Context) error {
	if len(a.scanPaths) > 0 {
		go func() {
			if _, err := a.scanner.ScanPaths(context.Background(), a.scanPaths); err != nil {
				a.logger.Warn("initial scan failed", "error", err)
			}
			// Newly scanned artists need avatars now, not at the next idle tick.
			if a.enricher != nil {
				a.enricher.Wake()
			}
		}()
	}
	if a.watch && len(a.scanPaths) > 0 {
		go func() {
			if err := a.watcher.Run(ctx); err != nil {
				a.logger.Warn("watcher stopped", "error", err)
			}
		}()
	}
	if a.onDemand != nil {
		go a.onDemand.Worker(ctx)
	}
	if a.federation != nil {
		go a.federation.Run(ctx)
	}
	if a.enricher != nil {
		// Short idle so incrementally-added artists are picked up promptly; the
		// post-scan Wake() handles the cold-start case immediately.
		go a.enricher.Run(ctx, 2*time.Minute)
	}
	if a.evictor != nil {
		// Always started; it self-gates on the runtime enabled flag.
		go a.evictor.Run(ctx)
	}
	if a.imports != nil {
		go a.imports.Worker(ctx)
	}

	srv := server.New(a.cfg.Server.Address, a.handler, a.logger)
	return srv.Run(ctx)
}

// providerControllerOrNil returns a nil interface (not a typed-nil) when the
// manager is absent, so the handler's `Providers == nil` check holds.
func providerControllerOrNil(m *core.ProviderManager) gossignol.ProviderController {
	if m == nil {
		return nil
	}
	return m
}

// Close releases resources.
func (a *App) Close() error {
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
