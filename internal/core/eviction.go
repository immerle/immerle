package core

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/immerle/immerle/internal/persistence"
)

// Evictor frees the disk space of provider-downloaded tracks that are no longer
// worth keeping. A file is evicted only when ALL of these hold (i.e. no reason to
// keep it): it was downloaded by a provider, it has not been played within the
// retention window, it is in no playlist, and neither it nor its album is starred
// by anyone. Only the file goes: the track row stays (as remote) with its stats,
// and is re-downloaded on the next play. Manually-added tracks are never touched.
//
// enabled and maxAge are read live (from the runtime settings) so the admin can
// change them without a restart; the loop in Run always runs but only sweeps
// while enabled.
type Evictor struct {
	catalog   *persistence.CatalogRepo
	downloads *persistence.DownloadRepo
	enabled   func() bool
	maxAge    func() time.Duration
	interval  time.Duration
	logger    *slog.Logger
}

// NewEvictor builds an Evictor. enabled/maxAge are read live; interval is the
// sweep cadence (read at boot; default 6h).
func NewEvictor(catalog *persistence.CatalogRepo, downloads *persistence.DownloadRepo, enabled func() bool, maxAge func() time.Duration, interval time.Duration, logger *slog.Logger) *Evictor {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	return &Evictor{catalog: catalog, downloads: downloads, enabled: enabled, maxAge: maxAge, interval: interval, logger: logger}
}

// Run sweeps on the configured interval until ctx is cancelled. Each tick is a
// no-op while the evictor is disabled, so it is safe to start unconditionally
// and toggle on later.
func (e *Evictor) Run(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		if e.enabled() {
			if removed, err := e.Sweep(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				e.logger.Warn("eviction sweep failed", "error", err)
			} else if removed > 0 {
				e.logger.Info("evicted unused provider downloads", "removed", removed)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Sweep deletes the files of eligible provider downloads, keeping their track
// rows as remote, and returns how many were evicted.
func (e *Evictor) Sweep(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-e.maxAge())
	candidates, err := e.catalog.ProviderTracksToEvict(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, t := range candidates {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		// Keep the row (id, annotations, playlists, album) and its job: only the
		// file goes, the next play re-downloads into this same track. Mark it
		// remote before deleting the file, so a scan or the file watcher seeing
		// the file vanish never mistakes it for a deleted track.
		if err := e.downloads.MarkEvicted(ctx, t.ID); err != nil {
			e.logger.Warn("could not mark evicted download", "track", t.ID, "error", err)
			continue
		}
		if err := e.catalog.MarkTrackEvicted(ctx, t.ID); err != nil {
			e.logger.Warn("could not mark evicted track", "track", t.ID, "error", err)
			continue
		}
		if t.Path != "" {
			if err := os.Remove(t.Path); err != nil && !os.IsNotExist(err) {
				// The row is already remote: the next play overwrites this file.
				e.logger.Warn("could not delete evicted file", "path", t.Path, "error", err)
			}
		}
		e.logger.Debug("evicted provider download", "track", t.ID, "path", t.Path)
		removed++
	}
	return removed, nil
}
