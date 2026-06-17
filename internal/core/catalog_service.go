package core

import (
	"context"

	"github.com/gossignol/gossignol/internal/models"
)

// CatalogService is the on-demand catalog (S5): it searches external providers,
// enqueues downloads and turns remote results into local tracks. Its runtime
// state and full method bodies are defined in ondemand.go.
type CatalogService struct {
	state *catalogServiceState
}

// catalogServiceState is declared in ondemand.go (S5).

// RemoteSearch returns streamable remote results for a query. It is wired into
// search3 so absent tracks still surface in client search.
func (s *CatalogService) RemoteSearch(ctx context.Context, query string, limit int) ([]models.Track, error) {
	if s == nil || s.state == nil {
		return nil, nil
	}
	return s.remoteSearch(ctx, query, limit)
}

// Resolve makes a (possibly remote) track available locally. It returns the
// resolved track, whether it is already local, and any download job id created.
func (s *CatalogService) Resolve(ctx context.Context, userID, trackID string) (models.Track, bool, string, error) {
	if s == nil || s.state == nil {
		return models.Track{}, false, "", nil
	}
	return s.resolve(ctx, userID, trackID)
}
