package core

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// PlaylistSyncEnqueuer queues a public playlist for federation-hub sync, and can
// purge all synced playlists from the hub (when sync is turned off). Implemented
// by the federation playlist syncer; optional (nil when federation is absent).
// Backed by the generic outbox, so it is fire-and-forget.
type PlaylistSyncEnqueuer interface {
	EnqueuePlaylistSync(ctx context.Context, playlistID string)
	PurgePlaylists(ctx context.Context) error
}

// PlaylistService holds the playlist CRUD business logic — visibility/edit
// permissions, create-vs-replace, metadata updates and ownership-aware delete —
// shared by every presentation layer.
type PlaylistService struct {
	playlists   *persistence.PlaylistRepo
	annotations *persistence.AnnotationRepo
	activity    *ActivityService     // optional
	hubSync     PlaylistSyncEnqueuer // optional
	onDemand    *CatalogService      // optional; resolves remote track ids before they hit playlist_tracks
}

// NewPlaylistService wires the playlist application service. activity, hubSync
// and onDemand are optional (pass nil when unused).
func NewPlaylistService(playlists *persistence.PlaylistRepo, annotations *persistence.AnnotationRepo, activity *ActivityService, hubSync PlaylistSyncEnqueuer, onDemand *CatalogService) *PlaylistService {
	return &PlaylistService{playlists: playlists, annotations: annotations, activity: activity, hubSync: hubSync, onDemand: onDemand}
}

// resolveTrackIDs maps each id through the on-demand resolver when it's a
// remote (not-yet-downloaded provider) track id, so playlist mutations only
// ever reference real local tracks — playlist_tracks has a foreign key on
// track_id, so inserting a remote id verbatim just fails with an opaque DB
// error deep in the persistence layer. Errors on the first id that can't be
// resolved (no on-demand resolver configured, or the search/download failed),
// naming it so the caller sees why, instead of adding it half-broken.
func (s *PlaylistService) resolveTrackIDs(ctx context.Context, userID string, ids []string) ([]string, error) {
	out := make([]string, len(ids))
	for i, id := range ids {
		if !IsRemoteID(id) {
			out[i] = id
			continue
		}
		if s.onDemand == nil {
			return nil, fmt.Errorf("track %s is not available locally", id)
		}
		track, _, _, err := s.onDemand.Resolve(ctx, userID, id)
		if err != nil || track.ID == "" {
			return nil, fmt.Errorf("resolve track %s: %w", id, err)
		}
		out[i] = track.ID
	}
	return out, nil
}

// enqueueHubSync queues a hub sync when federation is wired and the playlist is
// (or just stopped being) a local public playlist. The worker resolves whether
// that means an upsert or a delete from the playlist's current state.
func (s *PlaylistService) enqueueHubSync(ctx context.Context, p models.Playlist, wasPublic bool) {
	if s.hubSync == nil || p.Federated {
		return
	}
	if p.Public || wasPublic {
		s.hubSync.EnqueuePlaylistSync(ctx, p.ID)
	}
}

// PlaylistDetail is a playlist with its tracks. Track annotations are populated
// for reads (Get) and left nil for write responses (Create/Replace), matching
// the historical shapes.
type PlaylistDetail struct {
	Playlist models.Playlist
	Tracks   []TrackEntry
}

// PlaylistMetaUpdate carries the optional metadata changes for Update. Name is
// applied when non-empty; Comment when non-nil; PublicRaw is parsed as a bool
// and applied only when present and valid (an invalid value leaves it unchanged).
type PlaylistMetaUpdate struct {
	Name      string
	Comment   *string
	PublicRaw *string
}

// List returns the playlists visible to the user.
func (s *PlaylistService) List(ctx context.Context, userID string) ([]models.Playlist, error) {
	return s.playlists.ListVisible(ctx, userID)
}

// Get returns a playlist with its tracks (annotated for the caller). Returns
// persistence.ErrNotFound if it does not exist, ErrForbidden if not viewable.
func (s *PlaylistService) Get(ctx context.Context, user models.User, id string) (PlaylistDetail, error) {
	p, err := s.playlists.Get(ctx, id)
	if err != nil {
		return PlaylistDetail{}, err
	}
	if !s.canView(ctx, p, user) {
		return PlaylistDetail{}, ErrForbidden
	}
	tracks, err := s.playlists.Tracks(ctx, id)
	if err != nil {
		return PlaylistDetail{}, err
	}
	trackAnn, _ := s.annotations.AnnotationMap(ctx, user.ID, models.ItemTrack)
	return PlaylistDetail{Playlist: p, Tracks: toTrackEntries(tracks, trackAnn)}, nil
}

// Create makes a new playlist owned by the user, optionally seeding its tracks,
// and records "add" activity.
func (s *PlaylistService) Create(ctx context.Context, user models.User, name string, songIDs []string) (PlaylistDetail, error) {
	now := time.Now()
	p := models.Playlist{ID: uuid.NewString(), Name: name, OwnerID: user.ID, CreatedAt: now, UpdatedAt: now}
	if err := s.playlists.Create(ctx, p); err != nil {
		return PlaylistDetail{}, err
	}
	if len(songIDs) > 0 {
		resolved, err := s.resolveTrackIDs(ctx, user.ID, songIDs)
		if err != nil {
			return PlaylistDetail{}, err
		}
		if err := s.playlists.ReplaceTracks(ctx, p.ID, resolved, user.ID); err != nil {
			return PlaylistDetail{}, err
		}
	}
	if s.activity != nil {
		_ = s.activity.Record(ctx, user, "add", models.ItemPlaylist, p.ID)
	}
	return s.detail(ctx, p.ID)
}

// Replace overwrites an existing playlist's tracks (the createPlaylist call with
// a playlistId). Returns ErrForbidden if the user cannot edit it.
func (s *PlaylistService) Replace(ctx context.Context, user models.User, playlistID string, songIDs []string) (PlaylistDetail, error) {
	p, err := s.playlists.Get(ctx, playlistID)
	if err != nil {
		return PlaylistDetail{}, err
	}
	if !s.canEdit(ctx, p, user) {
		return PlaylistDetail{}, ErrForbidden
	}
	resolved, err := s.resolveTrackIDs(ctx, user.ID, songIDs)
	if err != nil {
		return PlaylistDetail{}, err
	}
	if err := s.playlists.ReplaceTracks(ctx, playlistID, resolved, user.ID); err != nil {
		return PlaylistDetail{}, err
	}
	s.enqueueHubSync(ctx, p, p.Public)
	return s.detail(ctx, playlistID)
}

// Update changes a playlist's metadata, appends tracks and removes tracks by
// index. Returns ErrForbidden if the user cannot edit it.
func (s *PlaylistService) Update(ctx context.Context, user models.User, id string, meta PlaylistMetaUpdate, addSongIDs []string, removeIndexes []int) error {
	p, err := s.playlists.Get(ctx, id)
	if err != nil {
		return err
	}
	if !s.canEdit(ctx, p, user) {
		return ErrForbidden
	}
	wasPublic := p.Public
	if meta.Name != "" {
		p.Name = meta.Name
	}
	if meta.Comment != nil {
		p.Comment = *meta.Comment
	}
	if meta.PublicRaw != nil {
		if b, err := strconv.ParseBool(*meta.PublicRaw); err == nil {
			p.Public = b
		}
	}
	if err := s.playlists.UpdateMeta(ctx, p); err != nil {
		return err
	}
	if len(addSongIDs) > 0 {
		resolved, err := s.resolveTrackIDs(ctx, user.ID, addSongIDs)
		if err != nil {
			return err
		}
		if err := s.playlists.AppendTracks(ctx, id, resolved, user.ID); err != nil {
			return err
		}
	}
	if len(removeIndexes) > 0 {
		if err := s.playlists.RemoveIndexes(ctx, id, removeIndexes); err != nil {
			return err
		}
	}
	s.enqueueHubSync(ctx, p, wasPublic)
	return nil
}

// Delete removes a playlist owned by the user (or by an admin). A non-owner —
// or anyone, for a federated playlist, whose nominal owner is just an internal
// attribution and never grants real ownership — is unsubscribed from it
// instead; if they were not subscribed, ErrForbidden.
func (s *PlaylistService) Delete(ctx context.Context, user models.User, id string) error {
	p, err := s.playlists.Get(ctx, id)
	if err != nil {
		return err
	}
	if p.Federated || (p.OwnerID != user.ID && !user.IsAdmin) {
		if ok, _ := s.playlists.Unsubscribe(ctx, id, user.ID); ok {
			return nil
		}
		return ErrForbidden
	}
	if err := s.playlists.Delete(ctx, id); err != nil {
		return err
	}
	s.enqueueHubSync(ctx, p, p.Public)
	return nil
}

// CoverTarget returns the playlist if the user may change its cover (owner or
// admin only — collaborators cannot; never for a federated playlist, read-only
// regardless of who its nominal owner is), for use before writing the cover file.
func (s *PlaylistService) CoverTarget(ctx context.Context, user models.User, id string) (models.Playlist, error) {
	p, err := s.playlists.Get(ctx, id)
	if err != nil {
		return models.Playlist{}, err
	}
	if p.Federated || (p.OwnerID != user.ID && !user.IsAdmin) {
		return models.Playlist{}, ErrForbidden
	}
	return p, nil
}

// SaveCover persists a playlist's custom cover id (call after CoverTarget has
// authorized and the cover file has been written).
func (s *PlaylistService) SaveCover(ctx context.Context, id, coverID string) error {
	if err := s.playlists.SetCover(ctx, id, coverID); err != nil {
		return err
	}
	if p, err := s.playlists.Get(ctx, id); err == nil {
		s.enqueueHubSync(ctx, p, p.Public)
	}
	return nil
}

// detail loads a playlist with its tracks, without per-user annotations (used
// for write responses).
func (s *PlaylistService) detail(ctx context.Context, id string) (PlaylistDetail, error) {
	p, err := s.playlists.Get(ctx, id)
	if err != nil {
		return PlaylistDetail{}, err
	}
	tracks, _ := s.playlists.Tracks(ctx, id)
	return PlaylistDetail{Playlist: p, Tracks: toTrackEntries(tracks, nil)}, nil
}

func (s *PlaylistService) canView(ctx context.Context, p models.Playlist, user models.User) bool {
	if p.OwnerID == user.ID || p.Public || user.IsAdmin {
		return true
	}
	collab, _ := s.playlists.IsCollaborator(ctx, p.ID, user.ID)
	return collab
}

func (s *PlaylistService) canEdit(ctx context.Context, p models.Playlist, user models.User) bool {
	if p.Federated {
		return false // federated playlists are read-only
	}
	if p.OwnerID == user.ID || user.IsAdmin {
		return true
	}
	if p.Collaborative {
		collab, _ := s.playlists.IsCollaborator(ctx, p.ID, user.ID)
		return collab
	}
	return false
}
