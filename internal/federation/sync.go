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
	"strings"
	"time"

	"github.com/immerle/immerle/internal/federation/hub"
	"github.com/immerle/immerle/internal/federation/stream"
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
	// maxMosaicCovers mirrors the client's PlaylistMosaic: at most the first 4
	// tracks (by position) contribute a cover.
	maxMosaicCovers = 4
	// mosaicPrefix marks a pre-resolution Image value as "compose a mosaic from
	// these cover ids" rather than a single stored cover id — see
	// playlistImageRef/resolveCovers.
	mosaicPrefix = "mosaic:"
)

// CoverSource resolves a cover id (playlist cover, track cover, or album id) to
// its original bytes + content type. Implemented by *stream.CoverService.
type CoverSource interface {
	Get(ctx context.Context, id string, size int, locale string) ([]byte, string, error)
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
	fed.SetReplayHandler(s.handleReplayRequest)
	return s
}

// EnqueuePlaylistSync queues a playlist for sync (implements
// core.PlaylistSyncEnqueuer); called on every public-playlist mutation. No-op
// when playlist sync is disabled in the runtime settings.
func (s *PlaylistSyncer) EnqueuePlaylistSync(ctx context.Context, playlistID string) {
	if s == nil || !s.fed.cfg().SyncPlaylists {
		return
	}
	if err := s.worker.Enqueue(ctx, PlaylistSyncKind, playlistID, ""); err != nil {
		s.logger.Warn("enqueue playlist sync failed", "playlist", playlistID, "error", err)
	}
}

// PurgePlaylists enqueues a delete for every playlist currently synced to the
// hub. Called when the operator turns playlist sync off, so the instance's
// playlists are removed from the hub rather than just left stale. Enqueues
// directly (bypassing the sync-enabled gate on EnqueuePlaylistSync).
func (s *PlaylistSyncer) PurgePlaylists(ctx context.Context) error {
	ids, err := s.syncState.IDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.worker.Enqueue(ctx, PlaylistSyncKind, id, ""); err != nil {
			return err
		}
	}
	return nil
}

// handle is the outbox.Handler: it defers while unlinked and maps a hub
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

// process resolves the playlist's current state and upserts or deletes it. A
// playlist is removed from the hub when it no longer qualifies — gone, private,
// federated, or playlist sync turned off entirely.
func (s *PlaylistSyncer) process(ctx context.Context, playlistID string) error {
	p, err := s.playlists.Get(ctx, playlistID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return s.deletePlaylist(ctx, playlistID)
		}
		return err
	}
	if !p.Public || p.Federated || !s.fed.cfg().SyncPlaylists {
		return s.deletePlaylist(ctx, playlistID)
	}
	return s.syncPlaylist(ctx, p)
}

func (s *PlaylistSyncer) deletePlaylist(ctx context.Context, playlistID string) error {
	// Fire-and-forget over the socket (RFC-socket-federation-client.md §7):
	// unlike the REST DELETE, there's no "already gone" response to special-case.
	if err := s.fed.stream.Send(ctx, stream.Frame{Type: stream.TypePlaylistDelete, ExternalID: playlistID}); err != nil {
		return outbox.ErrNotReady
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
	version := p.UpdatedAt.Format(time.RFC3339Nano)
	frame, err := buildUpsertFrame(p.ID, version, payload)
	if err != nil {
		return err
	}
	// Socket down: retried later (once reconnected) rather than falling back to
	// REST — see RFC §7. The hourly REST pull still catches up other instances
	// in the meantime.
	if err := s.fed.stream.Send(ctx, frame); err != nil {
		return outbox.ErrNotReady
	}
	resolved, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := s.syncState.SetPayload(ctx, p.ID, string(resolved), version); err != nil {
		return err
	}
	return s.syncState.Set(ctx, p.ID, contentHash)
}

// handleReplayRequest answers a replay.request (RFC §6): for every playlist
// still synced, replay its last resolved payload if the subscriber's cursor is
// older than it — no recomputation, PlaylistSyncRepo already has it (§8).
func (s *PlaylistSyncer) handleReplayRequest(ctx context.Context, f stream.Frame) error {
	ids, err := s.syncState.IDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		resolved, version, err := s.syncState.LastPayload(ctx, id)
		if err != nil {
			return err
		}
		if resolved == "" || version <= f.SinceVersion {
			continue // never pushed yet, or the subscriber already has this state
		}
		var payload syncPayload
		if err := json.Unmarshal([]byte(resolved), &payload); err != nil {
			s.logger.Warn("replay: corrupt stored payload, skipping", "playlist", id, "error", err)
			continue
		}
		frame, err := buildUpsertFrame(id, version, payload)
		if err != nil {
			return err
		}
		frame.Target = f.ForSubscriberID
		if err := s.fed.stream.Send(ctx, frame); err != nil {
			return err
		}
	}
	return nil
}

// buildUpsertFrame turns a resolved sync payload into the socket's
// playlist.upsert wire shape — name/description travel inside Metadata (the
// Frame itself has no dedicated fields for them, see streamMetadata).
func buildUpsertFrame(externalID, version string, p syncPayload) (stream.Frame, error) {
	metadata, err := json.Marshal(streamMetadata{Name: p.Name, Description: p.Description})
	if err != nil {
		return stream.Frame{}, err
	}
	tracks, err := json.Marshal(p.Tracks)
	if err != nil {
		return stream.Frame{}, err
	}
	return stream.Frame{
		Type: stream.TypePlaylistUpsert, ExternalID: externalID, Version: version,
		Image: p.Image, Tracks: tracks, Metadata: metadata,
	}, nil
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
		Image:       playlistImageRef(p, tracks), // pre-resolution ref; resolved to a hub URL below
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

// playlistImageRef is the pre-resolution Image value for the sync payload:
// the owner-chosen cover id when set, else a "mosaic:id1,id2,..." marker
// listing up to the first 4 tracks' covers — resolveCovers composes and
// uploads an actual mosaic image for it, the same one the client would render
// locally for a playlist with no custom cover. Empty when there's nothing to
// show a cover for at all.
func playlistImageRef(p models.Playlist, tracks []models.Track) string {
	if p.CoverArt != "" {
		return p.CoverArt
	}
	ids := mosaicCoverIDs(tracks)
	if len(ids) == 0 {
		return ""
	}
	return mosaicPrefix + strings.Join(ids, ",")
}

// mosaicCoverIDs returns the covers of up to the first maxMosaicCovers tracks
// by position, skipping tracks with no cover — mirrors coverArtsByPlaylist in
// internal/persistence/playlists.go (the client-side mosaic's data source).
func mosaicCoverIDs(tracks []models.Track) []string {
	ids := make([]string, 0, maxMosaicCovers)
	for _, t := range tracks {
		if len(ids) == maxMosaicCovers {
			break
		}
		if id := trackCoverID(t); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// mosaicIDsFromRef extracts the cover ids from a "mosaic:..." Image ref, or
// nil if ref isn't one (a plain cover id, or empty).
func mosaicIDsFromRef(ref string) []string {
	if !strings.HasPrefix(ref, mosaicPrefix) {
		return nil
	}
	return strings.Split(strings.TrimPrefix(ref, mosaicPrefix), ",")
}

// resolveCovers hashes every referenced cover, composes a mosaic image for a
// playlist with no custom cover (see playlistImageRef), uploads whatever the
// hub is missing (content-addressed de-dup), and rewrites image/track covers
// to hub URLs. Covers that can't be read or exceed the size cap are dropped.
func (s *PlaylistSyncer) resolveCovers(ctx context.Context, p *syncPayload) error {
	mosaicIDs := mosaicIDsFromRef(p.Image)

	ids := map[string]bool{}
	if p.Image != "" && mosaicIDs == nil {
		ids[p.Image] = true
	}
	for _, id := range mosaicIDs {
		ids[id] = true
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
		data, ct, err := s.covers.Get(ctx, id, 0, "")
		if err != nil || len(data) == 0 || len(data) > maxCoverBytes {
			continue
		}
		sum := sha256.Sum256(data)
		h := hex.EncodeToString(sum[:])
		idHash[id] = h
		blobs[h] = data
		ctypes[h] = ct
	}

	// Compose the mosaic from the tile covers just fetched above, and register
	// it under the marker itself (in the same idHash/blobs/ctypes maps the
	// upload pass below walks) so the coverURL lookup for p.Image at the end
	// resolves it exactly like any other cover. Silently skipped (p.Image ends
	// up unresolved → "") if none of the tile covers could be read/decoded.
	if len(mosaicIDs) > 0 {
		tiles := make([][]byte, 0, len(mosaicIDs))
		for _, id := range mosaicIDs {
			if h, ok := idHash[id]; ok {
				tiles = append(tiles, blobs[h])
			}
		}
		if mosaic, ct, err := renderMosaic(tiles); err == nil {
			sum := sha256.Sum256(mosaic)
			h := hex.EncodeToString(sum[:])
			idHash[p.Image] = h
			blobs[h] = mosaic
			ctypes[h] = ct
		}
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
