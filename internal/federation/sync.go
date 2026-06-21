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
	"github.com/immerle/immerle/internal/outbox"
	"github.com/immerle/immerle/internal/persistence"
)

// PlaylistSyncKind is the outbox job kind handled by the PlaylistSyncer.
const PlaylistSyncKind = "playlist_sync"

const (
	// hubRateInterval spaces out hub calls (~5 req/s) to respect the hub limit.
	hubRateInterval = 220 * time.Millisecond
	// maxCoverBytes is the hub's per-cover upload cap.
	maxCoverBytes = 5 << 20
	// rateLimitRetry is the retry delay after the hub rate-limits us (429).
	rateLimitRetry = time.Minute
)

// CoverSource resolves a cover id (playlist cover, track cover, or album id) to
// its original bytes + content type. Implemented by *stream.CoverService.
type CoverSource interface {
	Get(ctx context.Context, id string, size int) ([]byte, string, error)
}

// PlaylistSyncer is the outbox consumer that pushes public playlists to the
// federation hub (upsert) or removes them (delete), with a content-addressed
// cover de-dup pass. It registers itself as a handler on a generic outbox.Worker
// and is the enqueuer the PlaylistService calls on public-playlist mutations.
type PlaylistSyncer struct {
	fed        *Service
	worker     *outbox.Worker
	syncState  *persistence.PlaylistSyncRepo
	coverCache *persistence.CoverUploadRepo
	playlists  *persistence.PlaylistRepo
	covers     CoverSource
	logger     *slog.Logger
	lastCall   time.Time // hub-call rate gate (the worker drains single-threaded)
}

// NewPlaylistSyncer builds the syncer and registers its handler on worker for
// PlaylistSyncKind. It reuses the federation Service for the hub client,
// credentials and "are we linked?" check.
func NewPlaylistSyncer(fed *Service, worker *outbox.Worker, syncState *persistence.PlaylistSyncRepo, coverCache *persistence.CoverUploadRepo, playlists *persistence.PlaylistRepo, covers CoverSource, logger *slog.Logger) *PlaylistSyncer {
	s := &PlaylistSyncer{
		fed: fed, worker: worker, syncState: syncState, coverCache: coverCache,
		playlists: playlists, covers: covers, logger: logger,
	}
	worker.Register(PlaylistSyncKind, s.handle)
	return s
}

// EnqueuePlaylistSync queues a playlist for sync (implements
// core.PlaylistSyncEnqueuer); called on every public-playlist mutation.
func (s *PlaylistSyncer) EnqueuePlaylistSync(ctx context.Context, playlistID string) {
	if s == nil {
		return
	}
	if err := s.worker.Enqueue(ctx, PlaylistSyncKind, playlistID, ""); err != nil {
		s.logger.Warn("enqueue playlist sync failed", "playlist", playlistID, "error", err)
	}
}

// handle is the outbox.Handler: it defers while unlinked, and maps a hub
// rate-limit to an explicit retry delay; other errors get default backoff.
func (s *PlaylistSyncer) handle(ctx context.Context, job persistence.OutboxJob) error {
	if !s.fed.HubConfigured() {
		return outbox.ErrNotReady
	}
	if err := s.process(ctx, job.DedupeKey); err != nil {
		if isRateLimited(err) {
			return outbox.RetryAfter(rateLimitRetry, err)
		}
		return err
	}
	return nil
}

// process resolves the playlist's current state and upserts or deletes it.
func (s *PlaylistSyncer) process(ctx context.Context, playlistID string) error {
	p, err := s.playlists.Get(ctx, playlistID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return s.deletePlaylist(ctx, playlistID)
		}
		return err
	}
	// Only public, non-federated playlists belong on the hub.
	if !p.Public || p.Federated {
		return s.deletePlaylist(ctx, playlistID)
	}
	return s.syncPlaylist(ctx, p)
}

func (s *PlaylistSyncer) deletePlaylist(ctx context.Context, playlistID string) error {
	s.throttle()
	// A 404 means the playlist was never synced — treat it as already-gone.
	if err := s.fed.hub.DeletePlaylist(ctx, s.fed.auth(), playlistID); err != nil {
		var he *hub.HTTPError
		if !errors.As(err, &he) || he.Status != http.StatusNotFound {
			return err
		}
	}
	return s.syncState.Delete(ctx, playlistID)
}

func (s *PlaylistSyncer) syncPlaylist(ctx context.Context, p models.Playlist) error {
	tracks, err := s.playlists.Tracks(ctx, p.ID)
	if err != nil {
		return err
	}
	payload := buildPayload(p, tracks)

	// Skip entirely (no hub calls) when the logical content is unchanged. The
	// hash is over the payload with local cover ids in place, so a cover change
	// (cover id changes) also changes the hash.
	contentHash := hashPayload(payload)
	if prev, _ := s.syncState.Hash(ctx, p.ID); prev == contentHash {
		return nil
	}

	if err := s.resolveCovers(ctx, &payload); err != nil {
		return err
	}
	s.throttle()
	if err := s.fed.hub.SyncPlaylist(ctx, s.fed.auth(), p.ID, payload); err != nil {
		return err
	}
	return s.syncState.Set(ctx, p.ID, contentHash)
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
func (s *PlaylistSyncer) resolveCovers(ctx context.Context, p *syncPayload) error {
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
		data, ct, err := s.covers.Get(ctx, id, 0)
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
		unknown, err := s.coverCache.Unknown(ctx, hashes)
		if err != nil {
			return err
		}
		if len(unknown) > 0 {
			s.throttle()
			missing, err := s.fed.hub.MissingCovers(ctx, s.fed.auth(), unknown)
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
				s.throttle()
				if err := s.fed.hub.UploadCover(ctx, s.fed.auth(), h, ct, blobs[h]); err != nil {
					return err
				}
			}
			// Everything in `unknown` is now present (already-there + just-uploaded).
			_ = s.coverCache.Mark(ctx, unknown...)
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

func isRateLimited(err error) bool {
	var he *hub.HTTPError
	return errors.As(err, &he) && he.Status == http.StatusTooManyRequests
}

// throttle spaces successive hub calls to ~hubRateInterval. The outbox worker
// drains single-threaded, so a plain timestamp gate is enough.
func (s *PlaylistSyncer) throttle() {
	if !s.lastCall.IsZero() {
		if wait := hubRateInterval - time.Since(s.lastCall); wait > 0 {
			time.Sleep(wait)
		}
	}
	s.lastCall = time.Now()
}
