package core

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// ScrobbleEnqueuer submits a completed play to an external scrobbling service
// (ListenBrainz) for the given user, keyed on their own credentials. Backed by
// the generic outbox, so it is fire-and-forget. Implemented by
// *listenbrainz.Scrobbler; optional (nil when unset).
type ScrobbleEnqueuer interface {
	EnqueueScrobble(ctx context.Context, user models.User, track models.Track, at time.Time)
}

// PlaybackService holds the user-state mutations on library items — favorites,
// ratings and play scrobbles — shared by every presentation layer. Remote
// (provider) track ids are resolved to their local copy (downloaded on demand)
// so the state attaches to a real library row.
type PlaybackService struct {
	catalog     *persistence.CatalogRepo
	annotations *persistence.AnnotationRepo
	scrobbles   *persistence.ScrobbleRepo
	// Optional collaborators; may be nil when the feature is disabled.
	onDemand     *CatalogService
	activity     *ActivityService
	nowPlaying   *NowPlayingTracker
	scrobbleSync ScrobbleEnqueuer
}

// NewPlaybackService wires the playback/annotation application service. onDemand,
// activity, nowPlaying and scrobbleSync are optional and may be nil.
func NewPlaybackService(catalog *persistence.CatalogRepo, annotations *persistence.AnnotationRepo, scrobbles *persistence.ScrobbleRepo, onDemand *CatalogService, activity *ActivityService, nowPlaying *NowPlayingTracker, scrobbleSync ScrobbleEnqueuer) *PlaybackService {
	return &PlaybackService{catalog: catalog, annotations: annotations, scrobbles: scrobbles, onDemand: onDemand, activity: activity, nowPlaying: nowPlaying, scrobbleSync: scrobbleSync}
}

// SetStarred stars/unstars tracks, albums and artists. Track ids may be remote
// and are resolved to a local copy first. Records "favorite" activity when
// starring. Per-item failures are best-effort and not surfaced (as in the API).
func (s *PlaybackService) SetStarred(ctx context.Context, user models.User, trackIDs, albumIDs, artistIDs []string, star bool) {
	for _, rawID := range trackIDs {
		id := s.localTrackID(ctx, user.ID, rawID)
		_ = s.annotations.SetStarred(ctx, user.ID, models.ItemTrack, id, star)
		if star && s.activity != nil {
			_ = s.activity.Record(ctx, user, "favorite", models.ItemTrack, id)
		}
	}
	for _, id := range albumIDs {
		_ = s.annotations.SetStarred(ctx, user.ID, models.ItemAlbum, id, star)
		if star && s.activity != nil {
			_ = s.activity.Record(ctx, user, "favorite", models.ItemAlbum, id)
		}
	}
	for _, id := range artistIDs {
		_ = s.annotations.SetStarred(ctx, user.ID, models.ItemArtist, id, star)
	}
}

// SetRating rates the item the id refers to. A remote track id is rated on its
// local copy; otherwise the item type (album/artist/track) is detected.
func (s *PlaybackService) SetRating(ctx context.Context, userID, id string, rating int) error {
	if IsRemoteID(id) {
		return s.annotations.SetRating(ctx, userID, models.ItemTrack, s.localTrackID(ctx, userID, id), rating)
	}
	itemType := models.ItemTrack
	if _, err := s.catalog.GetAlbum(ctx, id); err == nil {
		itemType = models.ItemAlbum
	} else if _, err := s.catalog.GetArtist(ctx, id); err == nil {
		itemType = models.ItemArtist
	}
	return s.annotations.SetRating(ctx, userID, itemType, id, rating)
}

// Scrobble registers playback for each id: it sets the now-playing entry and,
// when submission is true and the user has scrobbling enabled, records a
// scrobble plus track/album play counts at the given time. Records "listen"
// activity for submissions. Remote ids are resolved to a local track first.
func (s *PlaybackService) Scrobble(ctx context.Context, user models.User, ids []string, submission bool, at time.Time) {
	for _, id := range ids {
		track, err := s.resolveTrack(ctx, user.ID, id)
		if err != nil {
			continue
		}
		if s.nowPlaying != nil {
			s.nowPlaying.Set(user.ID, user.Username, track.ID)
		}
		if !submission {
			continue
		}
		if user.ScrobbleEnabled {
			_ = s.scrobbles.Insert(ctx, models.Scrobble{
				ID:        uuid.NewString(),
				UserID:    user.ID,
				TrackID:   track.ID,
				PlayedAt:  at,
				Submitted: true,
			})
			_ = s.annotations.IncrementPlay(ctx, user.ID, models.ItemTrack, track.ID, at)
			_ = s.annotations.IncrementPlay(ctx, user.ID, models.ItemAlbum, track.AlbumID, at)
			if s.scrobbleSync != nil {
				s.scrobbleSync.EnqueueScrobble(ctx, user, track, at)
			}
		}
		if s.activity != nil {
			_ = s.activity.Record(ctx, user, "listen", models.ItemTrack, track.ID)
		}
	}
}

// resolveTrack fetches a local track, or for a remote (provider) id triggers the
// on-demand download and returns the resulting local track.
func (s *PlaybackService) resolveTrack(ctx context.Context, userID, id string) (models.Track, error) {
	if IsRemoteID(id) && s.onDemand != nil {
		track, _, _, err := s.onDemand.Resolve(ctx, userID, id)
		return track, err
	}
	return s.catalog.GetTrack(ctx, id)
}

// localTrackID maps a possibly-remote track id to its local id, downloading on
// demand if needed. Annotations must attach to the local track, otherwise they
// target a synthetic remote id no library row matches. Non-remote ids and
// failures pass through unchanged.
func (s *PlaybackService) localTrackID(ctx context.Context, userID, id string) string {
	if IsRemoteID(id) && s.onDemand != nil {
		if track, _, _, err := s.onDemand.Resolve(ctx, userID, id); err == nil && track.ID != "" {
			return track.ID
		}
	}
	return id
}
