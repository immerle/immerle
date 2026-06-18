package core

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/providers"
)

// ProviderRegistry holds the configured providers, keyed by name. It is safe
// for concurrent use: providers may be registered, replaced or removed at
// runtime (e.g. by the admin provider-management API) while searches and
// downloads read from it.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]providers.Provider
	order     []string
}

// NewProviderRegistry builds an empty registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{providers: map[string]providers.Provider{}}
}

// Register adds or replaces a provider.
func (r *ProviderRegistry) Register(p providers.Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[p.Name()]; !ok {
		r.order = append(r.order, p.Name())
	}
	r.providers[p.Name()] = p
}

// Unregister removes a provider by name. Reports whether it was present.
func (r *ProviderRegistry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[name]; !ok {
		return false
	}
	delete(r.providers, name)
	for i, n := range r.order {
		if n == name {
			r.order = slices.Delete(r.order, i, i+1)
			break
		}
	}
	return true
}

// Reorder arranges the registry to follow the given name order. Names present in
// the registry but missing from the list keep their relative order at the end;
// unknown names are ignored. This drives All() order and the search fallback.
func (r *ProviderRegistry) Reorder(names []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pos := make(map[string]int, len(names))
	for i, n := range names {
		pos[n] = i
	}
	sort.SliceStable(r.order, func(i, j int) bool {
		pi, oki := pos[r.order[i]]
		pj, okj := pos[r.order[j]]
		if oki && okj {
			return pi < pj
		}
		if oki != okj {
			return oki // listed names come before unlisted ones
		}
		return false // keep relative order otherwise (stable)
	})
}

// Get returns a provider by name.
func (r *ProviderRegistry) Get(name string) (providers.Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// All returns providers in registration order.
func (r *ProviderRegistry) All() []providers.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]providers.Provider, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.providers[n])
	}
	return out
}

// onDemandScanner is the subset of the scanner needed to ingest a downloaded file.
type onDemandScanner interface {
	ScanFile(ctx context.Context, path string) error
}

// ProviderSettings supplies the hot-reloadable on-demand behaviour. Read live on
// each use so the admin can change it at runtime without a restart. Implemented
// by *SettingsService (and StaticProviderSettings for tests / defaults).
type ProviderSettings interface {
	AutoDownloadOnPlay() bool
	SearchTimeout() time.Duration
}

// StaticProviderSettings is a fixed ProviderSettings (tests / fallback).
type StaticProviderSettings struct {
	AutoDownload bool
	Timeout      time.Duration
}

// AutoDownloadOnPlay implements ProviderSettings.
func (s StaticProviderSettings) AutoDownloadOnPlay() bool { return s.AutoDownload }

// SearchTimeout implements ProviderSettings (defaulting to 3s).
func (s StaticProviderSettings) SearchTimeout() time.Duration {
	if s.Timeout <= 0 {
		return 3 * time.Second
	}
	return s.Timeout
}

// catalogServiceState carries the on-demand catalog's runtime dependencies.
type catalogServiceState struct {
	catalog     *persistence.CatalogRepo
	downloads   *persistence.DownloadRepo
	registry    *ProviderRegistry
	scanner     onDemandScanner
	settings    ProviderSettings
	downloadDir string
	ffmpegPath  string
	logger      *slog.Logger

	// Remote-search performance: per-provider result cache (TTL) deduped by
	// singleflight. The overall timeout comes from settings (read live).
	searchTTL   time.Duration
	searchSF    singleflight.Group
	searchMu    sync.Mutex
	searchCache map[string]searchCacheEntry

	group  singleflight.Group
	wakeCh chan struct{}
}

type searchCacheEntry struct {
	at  time.Time
	val any
}

// CatalogServiceConfig configures the on-demand catalog.
type CatalogServiceConfig struct {
	Catalog   *persistence.CatalogRepo
	Downloads *persistence.DownloadRepo
	Registry  *ProviderRegistry
	Scanner   onDemandScanner
	// Settings supplies hot-reloadable behaviour (default provider, auto-download,
	// search timeout). Defaults to StaticProviderSettings{} when nil.
	Settings    ProviderSettings
	DownloadDir string
	FFmpegPath  string
	Logger      *slog.Logger
}

// NewCatalogService builds an on-demand CatalogService.
func NewCatalogService(cfg CatalogServiceConfig) *CatalogService {
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.Settings == nil {
		cfg.Settings = StaticProviderSettings{}
	}
	return &CatalogService{state: &catalogServiceState{
		catalog:     cfg.Catalog,
		downloads:   cfg.Downloads,
		registry:    cfg.Registry,
		scanner:     cfg.Scanner,
		settings:    cfg.Settings,
		downloadDir: cfg.DownloadDir,
		ffmpegPath:  cfg.FFmpegPath,
		logger:      cfg.Logger,
		searchTTL:   60 * time.Second,
		searchCache: map[string]searchCacheEntry{},
		wakeCh:      make(chan struct{}, 1),
	}}
}

const remotePrefix = "remote:"

// encodeRemoteID builds the synthetic id used for not-yet-downloaded tracks.
func encodeRemoteID(provider, providerTrackID string) string {
	return remotePrefix + provider + ":" + providerTrackID
}

// IsRemoteID reports whether id refers to a remote (provider) track.
func IsRemoteID(id string) bool { return strings.HasPrefix(id, remotePrefix) }

func decodeRemoteID(id string) (provider, providerTrackID string, ok bool) {
	if !strings.HasPrefix(id, remotePrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(id, remotePrefix)
	i := strings.IndexByte(rest, ':')
	if i < 0 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

// remoteSearch queries the single search provider (default, else first) and
// returns streamable-remote tracks, deduplicated and filtered against the local
// library (by MBID). Cached, deduplicated and time-bounded.
func (s *CatalogService) remoteSearch(ctx context.Context, query string, limit int) ([]models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	prov, ok := s.searchProvider()
	if !ok {
		return nil, nil
	}
	ctx, cancel := s.searchCtx(ctx)
	defer cancel()
	return s.remoteTracksFrom(ctx, prov, query, limit), nil
}

// remoteTracksFrom searches a single provider and returns streamable-remote
// tracks, deduplicated and filtered against the local library (by MBID and
// completed downloads). The context/timeout is set by the caller.
func (s *CatalogService) remoteTracksFrom(ctx context.Context, prov providers.Provider, query string, limit int) []models.Track {
	st := s.state
	results, err := s.cachedTrackSearch(ctx, prov, query, limit)
	if err != nil {
		st.logger.Warn("provider search failed", "provider", prov.Name(), "error", err)
		return nil
	}

	out := make([]models.Track, 0, limit)
	seen := make(map[string]bool)
	for _, res := range results {
		if len(out) >= limit {
			break
		}
		id := encodeRemoteID(prov.Name(), res.ProviderTrackID)
		if seen[id] {
			continue
		}
		// Skip results already present locally, so a downloaded track shows once
		// (as the local entry, with its stars/stats) instead of also re-appearing
		// as a fresh remote pointer: first by MBID, then by a completed download
		// of this exact provider track (the MBID dedup is a no-op for providers
		// like Deezer that carry no MBID).
		if res.MBID != "" {
			if _, exists, _ := st.catalog.TrackExistsByMBIDOrHash(ctx, res.MBID, ""); exists {
				continue
			}
		}
		if job, err := st.downloads.GetByProviderTrack(ctx, prov.Name(), res.ProviderTrackID); err == nil &&
			job.Status == models.DownloadCompleted && job.TrackID != "" {
			continue
		}
		seen[id] = true
		out = append(out, toRemoteTrack(prov.Name(), res))
	}
	return out
}

// resolve makes a track available locally, downloading it if needed. It returns
// the resolved local track, whether it is now local, and the download job id.
func (s *CatalogService) resolve(ctx context.Context, userID, trackID string) (models.Track, bool, string, error) {
	st := s.state
	if !IsRemoteID(trackID) {
		t, err := st.catalog.GetTrack(ctx, trackID)
		if err != nil {
			return models.Track{}, false, "", err
		}
		return t, !t.Remote, "", nil
	}

	provName, ptid, ok := decodeRemoteID(trackID)
	if !ok {
		return models.Track{}, false, "", fmt.Errorf("invalid remote id")
	}
	prov, ok := st.registry.Get(provName)
	if !ok {
		return models.Track{}, false, "", fmt.Errorf("unknown provider %q", provName)
	}

	// Deduplicate concurrent resolves of the same track. Detach from the first
	// caller's ctx: the coalesced resolve/download is shared by all waiters, so
	// it must survive that client disconnecting. (Stalls are bounded by the
	// provider HTTP client timeout.)
	v, err, _ := st.group.Do(trackID, func() (any, error) {
		return s.resolveOnce(context.WithoutCancel(ctx), userID, prov, ptid)
	})
	if err != nil {
		return models.Track{}, false, "", err
	}
	res := v.(resolveResult)
	return res.track, res.local, res.jobID, nil
}

type resolveResult struct {
	track models.Track
	local bool
	jobID string
}

func (s *CatalogService) resolveOnce(ctx context.Context, userID string, prov providers.Provider, ptid string) (resolveResult, error) {
	st := s.state

	meta, err := prov.Resolve(ctx, ptid)
	if err != nil {
		return resolveResult{}, err
	}

	// Strict dedup against the existing library by MBID.
	if meta.MBID != "" {
		if id, exists, _ := st.catalog.TrackExistsByMBIDOrHash(ctx, meta.MBID, ""); exists {
			t, err := st.catalog.GetTrack(ctx, id)
			return resolveResult{track: t, local: true}, err
		}
	}

	// If a completed job already produced a local track, reuse it.
	if job, err := st.downloads.GetByProviderTrack(ctx, prov.Name(), ptid); err == nil {
		if job.Status == models.DownloadCompleted && job.TrackID != "" {
			t, err := st.catalog.GetTrack(ctx, job.TrackID)
			if err == nil {
				return resolveResult{track: t, local: true, jobID: job.ID}, nil
			}
		}
	}

	now := time.Now()
	job, err := st.downloads.Enqueue(ctx, models.DownloadJob{
		ID:              uuid.NewString(),
		UserID:          userID,
		Provider:        prov.Name(),
		ProviderTrackID: ptid,
		Query:           meta.Title,
		Status:          models.DownloadQueued,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return resolveResult{}, err
	}

	trackID, err := s.processJob(ctx, job, prov, meta)
	if err != nil {
		_ = st.downloads.Fail(ctx, job.ID, err.Error(), false)
		return resolveResult{}, err
	}
	_ = st.downloads.Complete(ctx, job.ID, trackID)

	t, err := st.catalog.GetTrack(ctx, trackID)
	return resolveResult{track: t, local: true, jobID: job.ID}, err
}

// processJob downloads, tags and ingests a provider track, returning the local
// track id. The download+ingest is deduplicated across the inline (play-time)
// resolve path and the background worker via singleflight keyed by the provider
// track, so the same file is never downloaded twice in parallel (which could
// corrupt the output and break MBID-based dedup).
func (s *CatalogService) processJob(ctx context.Context, job models.DownloadJob, prov providers.Provider, meta providers.Result) (string, error) {
	key := "job:" + job.Provider + ":" + job.ProviderTrackID
	// Detach from the first caller's ctx so a disconnect doesn't cancel the
	// shared download/ingest for the other waiters coalesced on this key.
	v, err, _ := s.state.group.Do(key, func() (any, error) {
		return s.doProcessJob(context.WithoutCancel(ctx), job, prov, meta)
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (s *CatalogService) doProcessJob(ctx context.Context, job models.DownloadJob, prov providers.Provider, meta providers.Result) (string, error) {
	st := s.state
	suffix := meta.Suffix
	if suffix == "" {
		suffix = "mp3"
	}

	dest := s.destPath(meta, suffix)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}

	tmp := dest + ".part"
	if err := s.downloadTo(ctx, prov, job.ProviderTrackID, tmp); err != nil {
		os.Remove(tmp)
		return "", err
	}

	// Embed tags (and rename into place). Falls back to a plain rename.
	if err := s.embedTags(ctx, tmp, dest, meta); err != nil {
		os.Remove(tmp)
		return "", err
	}

	// Keep the cover art alongside the file (sidecar), so the downloaded track
	// shows the same artwork it had as a remote result.
	s.saveSidecarCover(ctx, meta, dest)

	// Ingest via the scanner so dedup/identity rules (S1) apply uniformly.
	if err := st.scanner.ScanFile(ctx, dest); err != nil {
		return "", err
	}
	return s.trackIDForPath(ctx, dest)
}

// trackIDForPath returns the local track id for a just-scanned file path.
func (s *CatalogService) trackIDForPath(ctx context.Context, dest string) (string, error) {
	paths, err := s.state.catalog.AllTrackPaths(ctx)
	if err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(dest)
	if id, ok := paths[abs]; ok {
		return id, nil
	}
	if id, ok := paths[dest]; ok {
		return id, nil
	}
	return "", fmt.Errorf("downloaded track not found after scan: %s", dest)
}

func (s *CatalogService) downloadTo(ctx context.Context, prov providers.Provider, ptid, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := prov.Download(ctx, ptid, f); err != nil {
		_ = f.Close()
		return err
	}
	// Closing flushes buffered writes; surface any error.
	return f.Close()
}

// embedTags writes ID3/metadata onto the downloaded file using ffmpeg (codec
// copy), placing the result at dest. If ffmpeg is unavailable it falls back to
// moving the file unchanged (tags already present from the provider are kept).
func (s *CatalogService) embedTags(ctx context.Context, src, dest string, meta providers.Result) error {
	args := []string{"-v", "error", "-nostdin", "-y", "-i", src, "-c", "copy",
		"-metadata", "title=" + meta.Title,
		"-metadata", "artist=" + meta.Artist,
		"-metadata", "album=" + meta.Album,
	}
	if meta.AlbumArtist != "" {
		args = append(args, "-metadata", "album_artist="+meta.AlbumArtist)
	}
	if meta.Genre != "" {
		args = append(args, "-metadata", "genre="+meta.Genre)
	}
	if meta.Year != 0 {
		args = append(args, "-metadata", "date="+strconv.Itoa(meta.Year))
	}
	if meta.TrackNo != 0 {
		args = append(args, "-metadata", "track="+strconv.Itoa(meta.TrackNo))
	}
	if meta.MBID != "" {
		args = append(args, "-metadata", "MUSICBRAINZ_TRACKID="+meta.MBID)
	}
	args = append(args, dest)

	cmd := exec.CommandContext(ctx, s.state.ffmpegPath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		s.state.logger.Warn("tag embed via ffmpeg failed, using raw file", "error", err, "output", string(out[:min(len(out), 300)]))
		return os.Rename(src, dest)
	}
	_ = os.Remove(src)
	return nil
}

// coverHTTPClient fetches provider cover images (server-side, like Download).
var coverHTTPClient = &http.Client{Timeout: 30 * time.Second}

// maxCoverBytes caps a downloaded cover image.
const maxCoverBytes = 8 << 20

// saveSidecarCover downloads the provider's cover art and writes it as a sidecar
// "cover.jpg" in the track's album folder, so the downloaded track keeps its
// artwork (the cover service serves album/track covers from this sidecar; the
// Subsonic response always advertises the album id as the cover id). Best effort:
// any failure is logged and ignored. Skipped when the album already has a cover
// (e.g. an earlier track of the same album).
func (s *CatalogService) saveSidecarCover(ctx context.Context, meta providers.Result, dest string) {
	if meta.CoverImageURL == "" {
		return
	}
	target := filepath.Join(filepath.Dir(dest), "cover.jpg")
	if _, err := os.Stat(target); err == nil {
		return
	}
	data, err := fetchImage(ctx, meta.CoverImageURL)
	if err != nil {
		s.state.logger.Warn("cover download failed", "url", meta.CoverImageURL, "error", err)
		return
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		s.state.logger.Warn("cover write failed", "path", target, "error", err)
	}
}

// fetchImage downloads url and returns its bytes only if they sniff as an image
// (so an HTML error page is never saved as a cover).
func fetchImage(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := coverHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cover status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCoverBytes))
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		return nil, fmt.Errorf("not an image")
	}
	return data, nil
}

// destPath builds Artist/Album/NN - Title.suffix under the download dir.
func (s *CatalogService) destPath(meta providers.Result, suffix string) string {
	artist := sanitize(firstNonEmpty(meta.AlbumArtist, meta.Artist, "Unknown Artist"))
	album := sanitize(firstNonEmpty(meta.Album, "Unknown Album"))
	title := sanitize(firstNonEmpty(meta.Title, "Unknown"))
	name := title
	if meta.TrackNo > 0 {
		name = fmt.Sprintf("%02d - %s", meta.TrackNo, title)
	}
	return filepath.Join(s.state.downloadDir, artist, album, name+"."+suffix)
}

// Worker runs the background download queue until ctx is cancelled. It also
// resumes jobs left in 'running' by a previous crash.
func (s *CatalogService) Worker(ctx context.Context) {
	st := s.state
	_ = st.downloads.RequeueStale(ctx)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		s.drainQueue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-st.wakeCh:
		case <-ticker.C:
		}
	}
}

func (s *CatalogService) drainQueue(ctx context.Context) {
	st := s.state
	for {
		job, err := st.downloads.ClaimNext(ctx)
		if err != nil {
			return // empty queue or error
		}
		prov, ok := st.registry.Get(job.Provider)
		if !ok {
			_ = st.downloads.Fail(ctx, job.ID, "unknown provider", false)
			continue
		}
		meta, err := prov.Resolve(ctx, job.ProviderTrackID)
		if err != nil {
			_ = st.downloads.Fail(ctx, job.ID, err.Error(), job.Attempts < 3)
			continue
		}
		trackID, err := s.processJob(ctx, job, prov, meta)
		if err != nil {
			_ = st.downloads.Fail(ctx, job.ID, err.Error(), job.Attempts < 3)
			continue
		}
		_ = st.downloads.Complete(ctx, job.ID, trackID)
	}
}

// EnqueueDownload queues a remote track for background download and wakes the worker.
func (s *CatalogService) EnqueueDownload(ctx context.Context, userID, trackID string) (string, error) {
	st := s.state
	provName, ptid, ok := decodeRemoteID(trackID)
	if !ok {
		return "", fmt.Errorf("not a remote track")
	}
	now := time.Now()
	job, err := st.downloads.Enqueue(ctx, models.DownloadJob{
		ID:              uuid.NewString(),
		UserID:          userID,
		Provider:        provName,
		ProviderTrackID: ptid,
		Status:          models.DownloadQueued,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return "", err
	}
	select {
	case st.wakeCh <- struct{}{}:
	default:
	}
	return job.ID, nil
}

// AutoDownloadOnPlay reports whether a remote track should be downloaded when
// first streamed.
func (s *CatalogService) AutoDownloadOnPlay() bool {
	return s != nil && s.state != nil && s.state.settings.AutoDownloadOnPlay()
}

func sanitize(s string) string {
	repl := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	out := strings.TrimSpace(repl.Replace(s))
	if out == "" {
		return "Unknown"
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
