package federation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/federation/hub"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// feedNamespace derives stable local ids for federated feed playlists, keyed by
// (source instance, instance-local playlist id), so re-syncs are idempotent and
// same-named playlists from different instances don't collide.
var feedNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("immerle:federation:feed"))

// feedPageSize is the feed page cap (the hub clamps to 50 regardless).
const feedPageSize = 50

// SetFeedOwnerResolver registers the lazy resolver for the virtual "system"
// account that owns playlists pulled from subscribed instances.
func (s *Service) SetFeedOwnerResolver(fn func(context.Context) (string, error)) { s.feedOwnerFn = fn }

// feedDetail is the subset of GET /instances/{id}/playlists/{externalId} we use.
// Tracks decode as the portable array immerle pushes on sync (stored opaquely by
// the hub) — not the swagger-"object" shape the generated client mistypes.
type feedDetail struct {
	OK       bool `json:"ok"`
	Playlist struct {
		ExternalID  string      `json:"externalId"`
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Tracks      []feedTrack `json:"tracks"`
	} `json:"playlist"`
}

type feedTrack struct {
	MBID   string `json:"mbid"`
	Artist string `json:"artist"`
	Title  string `json:"title"`
}

// SyncFeed pulls the playlists of every subscribed instance from the hub feed
// and materializes them as public, read-only playlists owned by the system
// account. It walks the full header feed each call (cheap, no tracks) and only
// fetches a playlist's tracks when its updatedAt changed since the last sync.
//
// ponytail: no prune — a playlist deleted on the source instance is kept
// locally. Add a feed-id reconciliation pass if stale copies become a problem.
func (s *Service) SyncFeed(ctx context.Context) error {
	if !s.HubConfigured() {
		return nil
	}
	owner, err := s.feedOwner(ctx)
	if err != nil {
		return err
	}
	if s.feedSeen == nil {
		s.feedSeen = map[string]string{}
	}

	after := ""
	for {
		s.feedThrottle()
		page, err := s.hub.FeedPlaylists(ctx, s.auth(), after, feedPageSize)
		if err != nil {
			return err
		}
		if page.Playlists != nil {
			for _, h := range *page.Playlists {
				if err := s.syncFeedPlaylist(ctx, owner, h); err != nil {
					s.logger.Warn("feed playlist sync failed", "name", deref(h.Name), "error", err)
				}
			}
		}
		if page.HasMore == nil || !*page.HasMore || page.NextUpdatedAfter == nil {
			return nil
		}
		after = *page.NextUpdatedAfter
	}
}

// syncFeedPlaylist materializes one feed header: it skips unchanged playlists,
// else fetches the full playlist (tracks) and upserts it locally.
func (s *Service) syncFeedPlaylist(ctx context.Context, owner string, h hub.PublicFeedPlaylistDTO) error {
	instanceID := ""
	if h.Author != nil {
		instanceID = deref(h.Author.Id)
	}
	externalID := deref(h.ExternalId)
	if instanceID == "" || externalID == "" {
		return nil
	}
	localID := uuid.NewSHA1(feedNamespace, []byte(instanceID+"/"+externalID)).String()
	updatedAt := deref(h.UpdatedAt)
	if updatedAt != "" && s.feedSeen[localID] == updatedAt {
		return nil
	}

	s.feedThrottle()
	var detail feedDetail
	if err := s.hub.InstancePlaylist(ctx, s.auth(), instanceID, externalID, &detail); err != nil {
		return err
	}

	var trackIDs []string
	for _, t := range detail.Playlist.Tracks {
		mbid, artist, title := t.MBID, t.Artist, t.Title
		if id, ok := s.resolveTrack(ctx, owner, hub.PublicDistributionTrack{Mbid: &mbid, Artist: &artist, Title: &title}); ok {
			trackIDs = append(trackIDs, id)
		}
	}

	if err := s.materializeFeed(ctx, localID, owner, detail); err != nil {
		return err
	}
	if err := s.playlists.ReplaceTracks(ctx, localID, trackIDs, ""); err != nil {
		return err
	}
	s.feedSeen[localID] = updatedAt
	return nil
}

// materializeFeed creates or updates the local playlist row (metadata only;
// tracks are replaced by the caller). It is public + federated (read-only).
func (s *Service) materializeFeed(ctx context.Context, localID, owner string, d feedDetail) error {
	existing, err := s.playlists.Get(ctx, localID)
	if err == nil {
		existing.Name = d.Playlist.Name
		existing.Comment = d.Playlist.Description
		existing.Public = true
		return s.playlists.UpdateMeta(ctx, existing)
	}
	if !errors.Is(err, persistence.ErrNotFound) {
		return err
	}
	now := time.Now()
	return s.playlists.Create(ctx, models.Playlist{
		ID:        localID,
		Name:      d.Playlist.Name,
		OwnerID:   owner,
		Comment:   d.Playlist.Description,
		Public:    true,
		Federated: true,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// feedOwner resolves (and caches) the system account that owns feed playlists.
func (s *Service) feedOwner(ctx context.Context) (string, error) {
	if s.feedOwnerID != "" {
		return s.feedOwnerID, nil
	}
	if s.feedOwnerFn == nil {
		return "", fmt.Errorf("federation: feed owner not configured")
	}
	id, err := s.feedOwnerFn(ctx)
	if err != nil {
		return "", err
	}
	s.feedOwnerID = id
	return id, nil
}

// feedThrottle spaces successive feed hub calls to ~hubRateInterval. SyncFeed
// runs on Run's single goroutine, so a plain timestamp gate is enough.
func (s *Service) feedThrottle() {
	if !s.lastFeedCall.IsZero() {
		if wait := hubRateInterval - time.Since(s.lastFeedCall); wait > 0 {
			time.Sleep(wait)
		}
	}
	s.lastFeedCall = time.Now()
}
