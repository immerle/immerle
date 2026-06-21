package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/immerle/immerle/internal/federation/hub"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

const (
	// outboxTick bounds how often the worker rescans the queue absent a wake.
	outboxTick = 15 * time.Second
	// hubRateInterval spaces out hub calls (~5 req/s) to respect the hub limit.
	hubRateInterval = 220 * time.Millisecond
	// maxCoverBytes is the hub's per-cover upload cap.
	maxCoverBytes = 5 << 20
	// maxSyncBackoff caps the retry delay for a failing job.
	maxSyncBackoff = 30 * time.Minute
)

// CoverSource resolves a cover id (playlist cover, track cover, or album id) to
// its original bytes + content type. Implemented by *stream.CoverService.
type CoverSource interface {
	Get(ctx context.Context, id string, size int) ([]byte, string, error)
}

// OutboxWorker drains the hub_outbox queue: it pushes public playlists to the
// federation hub (upsert) or removes them (delete), with retry/backoff and a
// content-addressed cover de-dup pass. A single worker runs in its own goroutine.
type OutboxWorker struct {
	fed        *Service
	outbox     *persistence.HubOutboxRepo
	syncState  *persistence.PlaylistSyncRepo
	coverCache *persistence.CoverUploadRepo
	playlists  *persistence.PlaylistRepo
	covers     CoverSource
	logger     *slog.Logger
	wake       chan struct{}
	lastCall   time.Time // hub-call rate gate (worker is single-threaded)
}

// NewOutboxWorker builds the playlist-sync worker. It reuses the federation
// Service for the hub client, credentials and "are we linked?" check.
func NewOutboxWorker(fed *Service, outbox *persistence.HubOutboxRepo, syncState *persistence.PlaylistSyncRepo, coverCache *persistence.CoverUploadRepo, playlists *persistence.PlaylistRepo, covers CoverSource, logger *slog.Logger) *OutboxWorker {
	return &OutboxWorker{
		fed: fed, outbox: outbox, syncState: syncState, coverCache: coverCache,
		playlists: playlists, covers: covers, logger: logger,
		wake: make(chan struct{}, 1),
	}
}

// EnqueuePlaylistSync queues a playlist for sync and wakes the worker. It
// implements core.HubSyncEnqueuer; called on every public-playlist mutation.
func (w *OutboxWorker) EnqueuePlaylistSync(ctx context.Context, playlistID string) {
	if w == nil {
		return
	}
	if err := w.outbox.Enqueue(ctx, playlistID); err != nil {
		w.logger.Warn("enqueue playlist sync failed", "playlist", playlistID, "error", err)
		return
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run drains the outbox while the instance is linked, waking on enqueue or tick.
func (w *OutboxWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(outboxTick)
	defer ticker.Stop()
	for {
		if w.fed.HubConfigured() {
			w.drain(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-ticker.C:
		}
	}
}

// drain processes every due job once. Failed jobs are rescheduled (future
// next_retry_at) so they are not re-claimed this round — the loop ends on the
// first ErrNotFound (queue empty / nothing due).
func (w *OutboxWorker) drain(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		job, err := w.outbox.ClaimNext(ctx, time.Now())
		if err != nil {
			return
		}
		if err := w.process(ctx, job.ExternalID); err != nil {
			delay := syncBackoff(job.Attempts, err)
			_ = w.outbox.Backoff(ctx, job.ExternalID, time.Now().Add(delay))
			w.logger.Warn("playlist sync failed; will retry", "playlist", job.ExternalID, "attempt", job.Attempts+1, "retryIn", delay, "error", err)
			if isRateLimited(err) {
				return // ease off until the next round
			}
			continue
		}
		_ = w.outbox.Done(ctx, job.ExternalID)
	}
}

// process resolves the playlist's current state and upserts or deletes it.
func (w *OutboxWorker) process(ctx context.Context, externalID string) error {
	p, err := w.playlists.Get(ctx, externalID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return w.deletePlaylist(ctx, externalID)
		}
		return err
	}
	// Only public, non-federated playlists belong on the hub.
	if !p.Public || p.Federated {
		return w.deletePlaylist(ctx, externalID)
	}
	return w.syncPlaylist(ctx, p)
}

func (w *OutboxWorker) deletePlaylist(ctx context.Context, externalID string) error {
	w.throttle()
	// A 404 means the playlist was never synced — treat it as already-gone.
	if err := w.fed.hub.DeletePlaylist(ctx, w.fed.auth(), externalID); err != nil {
		var he *hub.HTTPError
		if !errors.As(err, &he) || he.Status != http.StatusNotFound {
			return err
		}
	}
	return w.syncState.Delete(ctx, externalID)
}

func (w *OutboxWorker) syncPlaylist(ctx context.Context, p models.Playlist) error {
	tracks, err := w.playlists.Tracks(ctx, p.ID)
	if err != nil {
		return err
	}
	payload := buildPayload(p, tracks)

	// Skip entirely (no hub calls) when the logical content is unchanged. The
	// hash is over the payload with local cover ids in place, so a cover change
	// (cover id changes) also changes the hash.
	contentHash := hashPayload(payload)
	if prev, _ := w.syncState.Hash(ctx, p.ID); prev == contentHash {
		return nil
	}

	if err := w.resolveCovers(ctx, &payload); err != nil {
		return err
	}
	w.throttle()
	if err := w.fed.hub.SyncPlaylist(ctx, w.fed.auth(), p.ID, payload); err != nil {
		return err
	}
	return w.syncState.Set(ctx, p.ID, contentHash)
}

// syncPayload is the clean (typed) schema PUT to the hub. tracks is a JSON array,
// metadata a JSON object — both stored opaquely by the hub.
type syncPayload struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Image       string         `json:"image,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Tracks      []syncTrack    `json:"tracks"`
}

type syncTrack struct {
	MBID    string `json:"mbid,omitempty"`
	ISRC    string `json:"isrc,omitempty"` // not stored locally yet; always empty for now
	Artist  string `json:"artist,omitempty"`
	Title   string `json:"title,omitempty"`
	Album   string `json:"album,omitempty"`
	Cover   string `json:"cover,omitempty"` // local cover id pre-resolution, hub URL after
	Genre   string `json:"genre,omitempty"`
	Year    int    `json:"year,omitempty"`
	Seconds int    `json:"seconds,omitempty"`
}

func buildPayload(p models.Playlist, tracks []models.Track) syncPayload {
	out := syncPayload{
		Name:        p.Name,
		Description: p.Comment,
		Image:       p.CoverArt, // local cover id; resolved to a hub URL below
		Tracks:      make([]syncTrack, 0, len(tracks)),
	}
	if p.OwnerName != "" {
		out.Metadata = map[string]any{"owner": p.OwnerName}
	}
	for _, t := range tracks {
		out.Tracks = append(out.Tracks, syncTrack{
			MBID:    t.MBID,
			Artist:  t.ArtistName,
			Title:   t.Title,
			Album:   t.AlbumName,
			Cover:   trackCoverID(t),
			Genre:   t.Genre,
			Year:    t.Year,
			Seconds: t.Duration,
		})
	}
	return out
}

// trackCoverID is the effective cover id for a track (its own, else its album).
func trackCoverID(t models.Track) string {
	if t.CoverArt != "" {
		return t.CoverArt
	}
	return t.AlbumID
}

// resolveCovers hashes every referenced cover, uploads the ones the hub is
// missing (content-addressed de-dup), and rewrites image/track covers to hub
// URLs. Covers that can't be read or exceed the size cap are dropped.
func (w *OutboxWorker) resolveCovers(ctx context.Context, p *syncPayload) error {
	ids := map[string]bool{}
	if p.Image != "" {
		ids[p.Image] = true
	}
	for i := range p.Tracks {
		if p.Tracks[i].Cover != "" {
			ids[p.Tracks[i].Cover] = true
		}
	}

	idHash := map[string]string{} // cover id -> sha256 hex
	blobs := map[string][]byte{}  // hash -> bytes
	ctypes := map[string]string{} // hash -> content type
	for id := range ids {
		data, ct, err := w.covers.Get(ctx, id, 0)
		if err != nil || len(data) == 0 || len(data) > maxCoverBytes {
			continue
		}
		sum := sha256.Sum256(data)
		h := hex.EncodeToString(sum[:])
		idHash[id] = h
		blobs[h] = data
		ctypes[h] = ct
	}

	if len(idHash) > 0 {
		hset := map[string]bool{}
		for _, h := range idHash {
			hset[h] = true
		}
		hashes := make([]string, 0, len(hset))
		for h := range hset {
			hashes = append(hashes, h)
		}
		sort.Strings(hashes)

		// Only probe/upload covers we haven't already confirmed present.
		unknown, err := w.coverCache.Unknown(ctx, hashes)
		if err != nil {
			return err
		}
		if len(unknown) > 0 {
			w.throttle()
			missing, err := w.fed.hub.MissingCovers(ctx, w.fed.auth(), unknown)
			if err != nil {
				return err
			}
			missingSet := map[string]bool{}
			for _, h := range missing {
				missingSet[h] = true
			}
			for _, h := range unknown {
				if !missingSet[h] {
					continue
				}
				ct := ctypes[h]
				if ct == "" {
					ct = "image/jpeg"
				}
				w.throttle()
				if err := w.fed.hub.UploadCover(ctx, w.fed.auth(), h, ct, blobs[h]); err != nil {
					return err
				}
			}
			// Everything in `unknown` is now present (already-there + just-uploaded).
			_ = w.coverCache.Mark(ctx, unknown...)
		}
	}

	p.Image = coverURL(idHash, p.Image)
	for i := range p.Tracks {
		p.Tracks[i].Cover = coverURL(idHash, p.Tracks[i].Cover)
	}
	return nil
}

func coverURL(idHash map[string]string, id string) string {
	if h, ok := idHash[id]; ok {
		return "/api/v1/covers/" + h
	}
	return ""
}

func hashPayload(p syncPayload) string {
	b, _ := json.Marshal(p) // struct field order is fixed; map keys sorted → stable
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// syncBackoff is an exponential retry delay (5s, 10s, 20s … capped), with a
// 1-minute floor when the hub rate-limited us.
func syncBackoff(attempts int, err error) time.Duration {
	shift := attempts
	if shift > 8 {
		shift = 8
	}
	d := 5 * time.Second << uint(shift)
	if d <= 0 || d > maxSyncBackoff {
		d = maxSyncBackoff
	}
	if isRateLimited(err) && d < time.Minute {
		d = time.Minute
	}
	return d
}

func isRateLimited(err error) bool {
	var he *hub.HTTPError
	return errors.As(err, &he) && he.Status == http.StatusTooManyRequests
}

// throttle spaces successive hub calls to ~hubRateInterval (single worker).
func (w *OutboxWorker) throttle() {
	if !w.lastCall.IsZero() {
		if wait := hubRateInterval - time.Since(w.lastCall); wait > 0 {
			time.Sleep(wait)
		}
	}
	w.lastCall = time.Now()
}
