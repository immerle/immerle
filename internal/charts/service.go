package charts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// sourceInstanceID tags every playlist this package materializes, so they
// dedupe against (and never collide with) hub-editorial playlists (empty
// instance id) or subscribed-instance feed playlists (a real instance UUID) —
// see models.Playlist.SourceInstanceID.
const sourceInstanceID = "kworb"

// defaultInterval is the sync cadence: once a week.
const defaultInterval = 7 * 24 * time.Hour

// Service syncs DefaultCharts into public, read-only playlists using the same
// mechanism as hub-federated playlists (models.Playlist.Federated = true):
// tracks are kept by portable artist/title only (kworb has no MBID, only a
// Spotify track id we can't match against the local catalog), resolved
// lazily at play time exactly like an unmatched federated entry.
type Service struct {
	playlists *persistence.PlaylistRepo
	client    *client
	charts    []Chart
	interval  time.Duration
	coversDir string
	logger    *slog.Logger

	ownerID string
	ownerFn func(ctx context.Context) (string, error)
}

// New builds a Service. baseURL overrides the kworb data-set root (tests
// only; empty uses the real GitHub content). hc may be nil. coversDir is
// where generated chart covers are written (same directory as every other
// cover file); a newly-created chart playlist with no cover art is skipped
// (logged, not fatal) if empty.
func New(playlists *persistence.PlaylistRepo, baseURL, coversDir string, hc *http.Client, logger *slog.Logger) *Service {
	return &Service{
		playlists: playlists,
		client:    newClient(baseURL, hc),
		charts:    DefaultCharts,
		interval:  defaultInterval,
		coversDir: coversDir,
		logger:    logger,
	}
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
	return "", fmt.Errorf("charts: owner not configured")
}

// SyncNow fetches and upserts every configured chart, returning how many
// synced successfully. One chart failing (e.g. a transient fetch error) is
// logged and skipped rather than aborting the rest.
func (s *Service) SyncNow(ctx context.Context) (int, error) {
	ownerID, err := s.owner(ctx)
	if err != nil {
		return 0, err
	}
	synced := 0
	for _, c := range s.charts {
		if err := s.syncOne(ctx, ownerID, c); err != nil {
			s.logger.Warn("chart sync failed", "chart", c.Slug, "error", err)
			continue
		}
		synced++
	}
	return synced, nil
}

func (s *Service) syncOne(ctx context.Context, ownerID string, c Chart) error {
	raw, err := s.client.fetch(ctx, c.Slug)
	if err != nil {
		return err
	}

	entries := make([]persistence.FederatedTrackRef, 0, maxTracksPerChart)
	for i, e := range raw.Chart {
		if i >= maxTracksPerChart {
			break
		}
		artist, title := splitArtistAndTitle(e.ArtistAndTitle)
		if title == "" {
			continue
		}
		entries = append(entries, persistence.FederatedTrackRef{Artist: artist, Title: title})
	}

	sourceExternalID := c.Slug + "_weekly"
	existing, err := s.playlists.FindFederated(ctx, sourceInstanceID, sourceExternalID)
	switch {
	case err == nil:
		existing.Name = c.Name
		if err := s.playlists.UpdateMeta(ctx, existing); err != nil {
			return err
		}
		return s.playlists.ReplaceFederatedTracks(ctx, existing.ID, entries)
	case errors.Is(err, persistence.ErrNotFound):
		now := time.Now()
		p := models.Playlist{
			ID:               uuid.NewString(),
			Name:             c.Name,
			OwnerID:          ownerID,
			Public:           true,
			Federated:        true,
			SourceInstanceID: sourceInstanceID,
			SourceExternalID: sourceExternalID,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := s.playlists.Create(ctx, p); err != nil {
			return err
		}
		if coverID, err := s.storeCover(c.Slug); err != nil {
			s.logger.Warn("chart cover generation failed", "chart", c.Slug, "error", err)
		} else if err := s.playlists.SetCover(ctx, p.ID, coverID); err != nil {
			s.logger.Warn("chart cover save failed", "chart", c.Slug, "error", err)
		}
		return s.playlists.ReplaceFederatedTracks(ctx, p.ID, entries)
	default:
		return err
	}
}

// storeCover generates a flag/globe cover for slug and writes it under
// coversDir, returning the new cover id. Only called once, when the chart's
// playlist is first created — the cover is deterministic per slug, so
// there's nothing to refresh on subsequent syncs.
func (s *Service) storeCover(slug string) (string, error) {
	if s.coversDir == "" {
		return "", fmt.Errorf("charts: covers dir not configured")
	}
	data, err := generateCover(slug)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.coversDir, 0o755); err != nil {
		return "", err
	}
	coverID := uuid.NewString()
	if err := os.WriteFile(filepath.Join(s.coversDir, coverID), data, 0o644); err != nil {
		return "", err
	}
	return coverID, nil
}

// Run syncs on the configured interval until ctx is cancelled — once
// immediately, then every interval (weekly by default). Safe to start
// unconditionally at boot.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		if n, err := s.SyncNow(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Warn("chart sync run failed", "error", err)
		} else {
			s.logger.Info("chart sync complete", "synced", n, "total", len(s.charts))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
