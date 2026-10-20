// Package spotifyplaylists exposes public Spotify playlists as on-demand,
// read-only playlists. Nothing is imported: a playlist opened from search is
// mirrored as a federated playlist (the same mechanism as the chart playlists,
// see internal/charts) whose tracks are kept by artist/title only and resolved
// lazily at play time. A user keeps one by subscribing to it; a weekly refresh
// re-syncs those and drops the ones nobody kept.
package spotifyplaylists

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/spotifyweb"
	"github.com/immerle/immerle/internal/stream"
)

// ttl is how long a search result or a mirrored playlist is reused before
// Spotify is asked again.
const ttl = 7 * 24 * time.Hour

// checkInterval is how often Run looks for mirrored playlists older than ttl.
const checkInterval = 24 * time.Hour

// maxCachedSearches bounds the in-memory search cache.
const maxCachedSearches = 1000

type cachedSearch struct {
	at  time.Time
	res []spotifyweb.PlaylistSummary
}

// Service searches and mirrors Spotify playlists. Safe for concurrent use.
type Service struct {
	client    *spotifyweb.Client
	playlists *persistence.PlaylistRepo
	covers    *stream.CoverService
	enabled   func() bool
	ownerFn   func(ctx context.Context) (string, error)
	logger    *slog.Logger

	mu       sync.Mutex
	searches map[string]cachedSearch
}

// New builds a Service. covers caches the Spotify covers server-side (nil
// skips the cache sweep); enabled reports the live admin toggle; ownerFn
// resolves the nominal owner of mirrored playlists (the first admin).
func New(client *spotifyweb.Client, playlists *persistence.PlaylistRepo, covers *stream.CoverService, enabled func() bool, ownerFn func(ctx context.Context) (string, error), logger *slog.Logger) *Service {
	return &Service{client: client, playlists: playlists, covers: covers, enabled: enabled, ownerFn: ownerFn, logger: logger, searches: map[string]cachedSearch{}}
}

// Enabled reports whether the admin turned Spotify playlists on.
func (s *Service) Enabled() bool { return s.enabled() }

// Search returns up to limit public Spotify playlists matching query, reusing
// a result for ttl so repeated searches don't hit Spotify.
func (s *Service) Search(ctx context.Context, query string, limit int) ([]spotifyweb.PlaylistSummary, error) {
	key := fmt.Sprintf("%d|%s", limit, strings.ToLower(strings.TrimSpace(query)))
	s.mu.Lock()
	e, ok := s.searches[key]
	s.mu.Unlock()
	if ok && time.Since(e.at) < ttl {
		return e.res, nil
	}
	res, err := s.client.SearchPlaylists(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.searches) >= maxCachedSearches {
		s.searches = map[string]cachedSearch{} // ponytail: wholesale reset, an LRU if the hit rate ever matters
	}
	s.searches[key] = cachedSearch{at: time.Now(), res: res}
	return res, nil
}

// Open returns the local mirror of a Spotify playlist, (re)fetching it from
// Spotify only when it's missing or older than ttl.
func (s *Service) Open(ctx context.Context, spotifyID string) (models.Playlist, error) {
	p, err := s.playlists.FindFederated(ctx, persistence.SpotifySourceID, spotifyID)
	switch {
	case err == nil && time.Since(p.UpdatedAt) < ttl:
		return p, nil
	case err == nil:
		return s.sync(ctx, &p, spotifyID)
	case errors.Is(err, persistence.ErrNotFound):
		return s.sync(ctx, nil, spotifyID)
	default:
		return models.Playlist{}, err
	}
}

// sync fetches spotifyID from Spotify and writes it over existing (or a new
// mirror when nil). Already-resolved tracks keep their local track id.
func (s *Service) sync(ctx context.Context, existing *models.Playlist, spotifyID string) (models.Playlist, error) {
	src, err := s.client.FetchPlaylist(ctx, spotifyID)
	if err != nil {
		return models.Playlist{}, err
	}
	entries := make([]persistence.FederatedTrackRef, 0, len(src.Tracks))
	for _, t := range src.Tracks {
		entries = append(entries, persistence.FederatedTrackRef{Artist: t.Artist, Title: t.Title, Album: t.Album})
	}
	name := src.Name
	if name == "" {
		name = "Spotify"
	}

	var p models.Playlist
	if existing != nil {
		p = *existing
		p.Name = name
		if err := s.playlists.UpdateMeta(ctx, p); err != nil {
			return models.Playlist{}, err
		}
	} else {
		ownerID, err := s.ownerFn(ctx)
		if err != nil {
			return models.Playlist{}, err
		}
		now := time.Now()
		p = models.Playlist{
			ID:               uuid.NewString(),
			Name:             name,
			OwnerID:          ownerID,
			Public:           true,
			Federated:        true,
			SourceInstanceID: persistence.SpotifySourceID,
			SourceExternalID: spotifyID,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := s.playlists.Create(ctx, p); err != nil {
			return models.Playlist{}, err
		}
	}
	if cover := models.RemoteCoverID(src.ImageURL); cover != "" && cover != p.CoverArt {
		if err := s.playlists.SetCover(ctx, p.ID, cover); err != nil {
			s.logger.Warn("spotify playlist cover save failed", "spotifyId", spotifyID, "error", err)
		}
	}
	if err := s.playlists.ReplaceFederatedTracks(ctx, p.ID, entries); err != nil {
		return models.Playlist{}, err
	}
	return s.playlists.Get(ctx, p.ID)
}

// Refresh re-syncs every mirrored playlist older than ttl that a user kept
// (subscribed to) and deletes the others, then drops the cached Spotify covers
// older than ttl that no mirror uses anymore (dropped playlists, replaced
// covers, search results never opened). One failure is logged and skipped.
func (s *Service) Refresh(ctx context.Context) (refreshed, dropped int, err error) {
	stale, err := s.playlists.StaleFederated(ctx, persistence.SpotifySourceID, time.Now().Add(-ttl))
	if err != nil {
		return 0, 0, err
	}
	defer s.pruneCovers(ctx)
	for _, ref := range stale {
		if !ref.Subscribed {
			if err := s.playlists.Delete(ctx, ref.ID); err != nil {
				s.logger.Warn("spotify playlist drop failed", "id", ref.ID, "error", err)
				continue
			}
			dropped++
			continue
		}
		p, err := s.playlists.Get(ctx, ref.ID)
		if err == nil {
			_, err = s.sync(ctx, &p, ref.ExternalID)
		}
		if err != nil {
			s.logger.Warn("spotify playlist refresh failed", "spotifyId", ref.ExternalID, "error", err)
			continue
		}
		refreshed++
	}
	return refreshed, dropped, nil
}

// pruneCovers deletes cached Spotify covers older than ttl that no mirrored
// playlist references.
func (s *Service) pruneCovers(ctx context.Context) {
	if s.covers == nil {
		return
	}
	inUse, err := s.playlists.FederatedCovers(ctx, persistence.SpotifySourceID)
	if err != nil {
		s.logger.Warn("spotify cover sweep failed", "error", err)
		return
	}
	n := s.covers.PruneRemote(time.Now().Add(-ttl), func(id string) bool {
		return !inUse[id] && isSpotifyCover(id)
	})
	if n > 0 {
		s.logger.Info("spotify cover sweep complete", "removed", n)
	}
}

// isSpotifyCover reports whether a remote cover id points at a Spotify CDN.
func isSpotifyCover(id string) bool {
	raw, ok := models.DecodeRemoteCoverID(id)
	if !ok {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "scdn.co" || strings.HasSuffix(host, ".scdn.co") || host == "spotifycdn.com" || strings.HasSuffix(host, ".spotifycdn.com")
}

// Run calls Refresh daily (so each kept playlist is re-synced about once a
// week) until ctx is cancelled. Skipped while the feature is off.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		if s.enabled() {
			if refreshed, dropped, err := s.Refresh(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				s.logger.Warn("spotify playlist refresh run failed", "error", err)
			} else if refreshed+dropped > 0 {
				s.logger.Info("spotify playlist refresh complete", "refreshed", refreshed, "dropped", dropped)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
