package core

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/gossignol/gossignol/internal/persistence"
)

// Evictor garbage-collects provider-downloaded tracks that are no longer worth
// keeping. A track is removed only when ALL of these hold (i.e. no reason to
// keep it): it was downloaded by a provider, it has not been played within the
// retention window, it is in no playlist, and it is starred by nobody.
// Manually-added tracks are never touched.
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

// Sweep deletes eligible provider downloads (file + track row + download job)
// and returns how many were removed.
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
		if t.Path != "" {
			if err := os.Remove(t.Path); err != nil && !os.IsNotExist(err) {
				e.logger.Warn("could not delete evicted file", "path", t.Path, "error", err)
				// Still drop the DB rows so we don't keep retrying a missing file.
			}
		}
		if err := e.catalog.DeleteTrack(ctx, t.ID); err != nil {
			e.logger.Warn("could not delete evicted track", "track", t.ID, "error", err)
			continue
		}
		_ = e.downloads.DeleteByTrack(ctx, t.ID)
		e.logger.Debug("evicted provider download", "track", t.ID, "path", t.Path)
		removed++
	}
	return removed, nil
}
