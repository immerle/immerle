// Package autoplaylists materializes genre and decade playlists ("Rock",
// "Rap", "1990s"...) from the local catalog, refreshed periodically — the
// same public/federated-style materializer mechanism internal/charts uses for
// curated chart playlists, just sourced from the local library instead of a
// remote chart feed.
package autoplaylists

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// Source instance ids distinguish the two kinds in the dedupe key
// (FindFederated), so a genre and a decade never collide even if a genre
// happened to be named "1990s".
const (
	sourceGenre  = "genre-mix"
	sourceDecade = "decade-mix"
)

// minTracks is the minimum catalog size for a genre/decade to get its own
// playlist — below this it'd be a near-empty, not-worth-it playlist.
const minTracks = 15

// maxTracks bounds how many tracks a single auto-playlist holds.
const maxTracks = 100

// defaultInterval is the sync cadence. Daily (not weekly, like charts): unlike
// a remote chart feed, the local catalog changes on every scan, so genre/decade
// membership can shift the same day tracks are added.
const defaultInterval = 24 * time.Hour

// Service syncs genre and decade auto-playlists into public, read-only
// playlists using the same mechanism as curated chart playlists
// (models.Playlist.Federated = true) — except tracks are already local, so
// they're set directly by id (PlaylistRepo.ReplaceTracks), no artist/title
// resolution needed.
type Service struct {
	catalog   *persistence.CatalogRepo
	genres    *persistence.GenreRepo
	playlists *persistence.PlaylistRepo
	interval  time.Duration
	logger    *slog.Logger

	ownerID string
	ownerFn func(ctx context.Context) (string, error)
}

// New builds a Service.
func New(catalog *persistence.CatalogRepo, genres *persistence.GenreRepo, playlists *persistence.PlaylistRepo, logger *slog.Logger) *Service {
	return &Service{catalog: catalog, genres: genres, playlists: playlists, interval: defaultInterval, logger: logger}
}

// SetOwner pins the nominal owner of synced playlists (mainly for tests).
func (s *Service) SetOwner(id string) { s.ownerID = id }

// SetOwnerResolver lazily resolves (and caches) the nominal owner — typically
// the first admin account — so this works even before one exists at boot.
func (s *Service) SetOwnerResolver(fn func(ctx context.Context) (string, error)) {
	s.ownerFn = fn
}

func (s *Service) owner(ctx context.Context) (string, error) {
	if s.ownerID != "" {
		return s.ownerID, nil
	}
	if s.ownerFn != nil {
		id, err := s.ownerFn(ctx)
		if err != nil {
			return "", err
		}
		s.ownerID = id
		return id, nil
	}
	return "", fmt.Errorf("autoplaylists: owner not configured")
}

// SyncNow rebuilds every genre and decade auto-playlist, returning how many
// synced successfully (one genre or decade failing is logged and skipped
// rather than aborting the rest).
func (s *Service) SyncNow(ctx context.Context) (int, error) {
	ownerID, err := s.owner(ctx)
	if err != nil {
		return 0, err
	}
	synced := 0
	synced += s.syncGenres(ctx, ownerID)
	synced += s.syncDecades(ctx, ownerID)
	return synced, nil
}

func (s *Service) syncGenres(ctx context.Context, ownerID string) int {
	genres, err := s.genres.List(ctx)
	if err != nil {
		s.logger.Warn("autoplaylists: list genres failed", "error", err)
		return 0
	}
	synced := 0
	for _, g := range genres {
		if g.Name == "" || g.SongCount < minTracks {
			continue
		}
		tracks, err := s.catalog.ListTracksByGenre(ctx, g.Name, maxTracks, 0)
		if err != nil {
			s.logger.Warn("autoplaylists: list genre tracks failed", "genre", g.Name, "error", err)
			continue
		}
		if err := s.upsert(ctx, ownerID, sourceGenre, g.Name, g.Name, trackIDs(tracks)); err != nil {
			s.logger.Warn("autoplaylists: genre sync failed", "genre", g.Name, "error", err)
			continue
		}
		synced++
	}
	return synced
}

func (s *Service) syncDecades(ctx context.Context, ownerID string) int {
	synced := 0
	for _, d := range decades() {
		tracks, err := s.catalog.ListTracksByYearRange(ctx, d.from, d.from+10, maxTracks, 0)
		if err != nil {
			s.logger.Warn("autoplaylists: list decade tracks failed", "decade", d.label, "error", err)
			continue
		}
		if len(tracks) < minTracks {
			continue
		}
		if err := s.upsert(ctx, ownerID, sourceDecade, d.label, d.label, trackIDs(tracks)); err != nil {
			s.logger.Warn("autoplaylists: decade sync failed", "decade", d.label, "error", err)
			continue
		}
		synced++
	}
	return synced
}

// decade is one candidate decade bucket: [from, from+10).
type decade struct {
	from  int
	label string
}

// decades enumerates every decade from the 1950s through the current one —
// ListTracksByYearRange returning too few tracks for a given decade (see
// minTracks in syncDecades) naturally skips ones the local catalog has
// nothing in, so there's no need to first query which years are present.
func decades() []decade {
	var out []decade
	for from := 1950; from <= (time.Now().Year()/10)*10; from += 10 {
		out = append(out, decade{from: from, label: fmt.Sprintf("%ds", from)})
	}
	return out
}

func trackIDs(tracks []models.Track) []string {
	ids := make([]string, len(tracks))
	for i, t := range tracks {
		ids[i] = t.ID
	}
	return ids
}

// upsert creates or refreshes the auto-playlist for (sourceInstanceID,
// sourceExternalID), mirroring internal/charts.syncOne's find-or-create shape.
func (s *Service) upsert(ctx context.Context, ownerID, sourceInstanceID, sourceExternalID, name string, ids []string) error {
	cover := models.GeneratorCoverID(coverParams(name))
	existing, err := s.playlists.FindFederated(ctx, sourceInstanceID, sourceExternalID)
	switch {
	case err == nil:
		existing.Name = name
		if err := s.playlists.UpdateMeta(ctx, existing); err != nil {
			return err
		}
		if existing.CoverArt != cover {
			if err := s.playlists.SetCover(ctx, existing.ID, cover); err != nil {
				s.logger.Warn("autoplaylists: cover update failed", "name", name, "error", err)
			}
		}
		return s.playlists.ReplaceTracks(ctx, existing.ID, ids, ownerID)
	case errors.Is(err, persistence.ErrNotFound):
		now := time.Now()
		p := models.Playlist{
			ID: uuid.NewString(), Name: name, OwnerID: ownerID, Public: true, Federated: true,
			SourceInstanceID: sourceInstanceID, SourceExternalID: sourceExternalID,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.playlists.Create(ctx, p); err != nil {
			return err
		}
		if err := s.playlists.SetCover(ctx, p.ID, cover); err != nil {
			s.logger.Warn("autoplaylists: cover save failed", "name", name, "error", err)
		}
		return s.playlists.ReplaceTracks(ctx, p.ID, ids, ownerID)
	default:
		return err
	}
}

// coverGradients cycles a small fixed palette across genres/decades — picked
// deterministically by name so a given genre/decade always gets the same
// look across resyncs, without needing a per-genre color mapping (genres are
// open-ended free-text tags, no fixed list to map).
var coverGradients = [][2]string{
	{"#1db954", "#0b3d20"},
	{"#5b21b6", "#1e1b4b"},
	{"#be123c", "#4c0519"},
	{"#0369a1", "#0c2340"},
	{"#b45309", "#451a03"},
	{"#0f766e", "#042f2e"},
}

// coverParams builds the generator-cover query values (see
// internal/models.GeneratorCoverID and GET /cover/generator) for an
// auto-playlist: a generic music-note icon (genres are free-text, with no
// fixed emoji mapping) over a gradient picked from coverGradients by name.
func coverParams(name string) url.Values {
	sum := 0
	for _, r := range name {
		sum += int(r)
	}
	g := coverGradients[sum%len(coverGradients)]

	vals := url.Values{}
	vals.Set("icon", "1f3b5") // 🎵
	vals.Set("title", name)
	vals.Set("color", g[0])
	vals.Set("color2", g[1])
	vals.Set("angle", "45")
	return vals
}

// Run syncs on the configured interval until ctx is cancelled — once
// immediately, then every interval (daily by default).
func (s *Service) Run(ctx context.Context) {
	if _, err := s.SyncNow(ctx); err != nil {
		s.logger.Warn("autoplaylists: initial sync failed", "error", err)
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.SyncNow(ctx); err != nil {
				s.logger.Warn("autoplaylists: sync failed", "error", err)
			}
		}
	}
}
