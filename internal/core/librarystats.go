package core

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// LibraryStatsService keeps an in-memory snapshot of the library analytics
// (counts plus total on-disk size and duration). The snapshot is recomputed once
// at startup and after each scan, so the analytics endpoint serves it without
// touching the database — avoiding a SUM over every track on each request.
type LibraryStatsService struct {
	catalog *persistence.CatalogRepo
	logger  *slog.Logger

	mu     sync.RWMutex
	cached models.LibraryStats
}

// NewLibraryStatsService builds the service. Call Refresh once at boot to warm
// the cache.
func NewLibraryStatsService(catalog *persistence.CatalogRepo, logger *slog.Logger) *LibraryStatsService {
	return &LibraryStatsService{catalog: catalog, logger: logger}
}

// Refresh recomputes the snapshot from the catalog and stores it. Safe to call
// from a scan-completion hook.
func (s *LibraryStatsService) Refresh(ctx context.Context) (models.LibraryStats, error) {
	if s == nil {
		return models.LibraryStats{}, nil
	}
	artists, albums, tracks, err := s.catalog.Stats(ctx)
	if err != nil {
		return models.LibraryStats{}, err
	}
	size, duration, err := s.catalog.Totals(ctx)
	if err != nil {
		return models.LibraryStats{}, err
	}
	stats := models.LibraryStats{
		Artists:       artists,
		Albums:        albums,
		Tracks:        tracks,
		TotalSize:     size,
		TotalDuration: duration,
		UpdatedAt:     time.Now(),
	}
	s.mu.Lock()
	s.cached = stats
	s.mu.Unlock()
	return stats, nil
}

// Get returns the last computed snapshot (zero value before the first Refresh).
func (s *LibraryStatsService) Get() models.LibraryStats {
	if s == nil {
		return models.LibraryStats{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cached
}
