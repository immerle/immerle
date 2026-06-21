package core

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// HubSyncEnqueuer queues a public playlist for federation-hub sync. Implemented
// by the federation outbox worker; optional (nil when federation is absent).
type HubSyncEnqueuer interface {
	EnqueuePlaylistSync(ctx context.Context, playlistID string)
}

// PlaylistService holds the playlist CRUD business logic — visibility/edit
// permissions, create-vs-replace, metadata updates and ownership-aware delete —
// shared by every presentation layer.
type PlaylistService struct {
	playlists   *persistence.PlaylistRepo
	annotations *persistence.AnnotationRepo
	activity    *ActivityService // optional
	hubSync     HubSyncEnqueuer  // optional
}

// NewPlaylistService wires the playlist application service. activity and hubSync
// are optional (pass nil when unused).
func NewPlaylistService(playlists *persistence.PlaylistRepo, annotations *persistence.AnnotationRepo, activity *ActivityService, hubSync HubSyncEnqueuer) *PlaylistService {
	return &PlaylistService{playlists: playlists, annotations: annotations, activity: activity, hubSync: hubSync}
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
		_ = s.playlists.ReplaceTracks(ctx, p.ID, songIDs, user.ID)
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
	if err := s.playlists.ReplaceTracks(ctx, playlistID, songIDs, user.ID); err != nil {
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
		if err := s.playlists.AppendTracks(ctx, id, addSongIDs, user.ID); err != nil {
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

// Delete removes a playlist owned by the user (or by an admin). A non-owner is
// unsubscribed from it instead; if they were not subscribed, ErrForbidden.
func (s *PlaylistService) Delete(ctx context.Context, user models.User, id string) error {
	p, err := s.playlists.Get(ctx, id)
	if err != nil {
		return err
	}
	if p.OwnerID != user.ID && !user.IsAdmin {
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
// admin only — collaborators cannot), for use before writing the cover file.
func (s *PlaylistService) CoverTarget(ctx context.Context, user models.User, id string) (models.Playlist, error) {
	p, err := s.playlists.Get(ctx, id)
	if err != nil {
		return models.Playlist{}, err
	}
	if p.OwnerID != user.ID && !user.IsAdmin {
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
