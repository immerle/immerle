package core

import (
	"context"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// PlayQueueService holds the saved play-queue sync logic shared by every
// presentation layer: persisting a user's queue and resolving it back into its
// tracks.
type PlayQueueService struct {
	playQueues  *persistence.PlayQueueRepo
	catalog     *persistence.CatalogRepo
	annotations *persistence.AnnotationRepo
}

// NewPlayQueueService wires the play-queue application service.
func NewPlayQueueService(playQueues *persistence.PlayQueueRepo, catalog *persistence.CatalogRepo, annotations *persistence.AnnotationRepo) *PlayQueueService {
	return &PlayQueueService{playQueues: playQueues, catalog: catalog, annotations: annotations}
}

// PlayQueueResult is a saved queue resolved into its tracks.
type PlayQueueResult struct {
	Queue   models.PlayQueue
	Entries []TrackEntry
}

// Get returns the user's saved queue with its tracks. Returns
// persistence.ErrNotFound when no queue has been saved.
func (s *PlayQueueService) Get(ctx context.Context, userID string) (PlayQueueResult, error) {
	q, err := s.playQueues.Get(ctx, userID)
	if err != nil {
		return PlayQueueResult{}, err
	}
	trackAnn, _ := s.annotations.AnnotationMap(ctx, userID, models.ItemTrack)
	entries := make([]TrackEntry, 0, len(q.TrackIDs))
	for _, id := range q.TrackIDs {
		t, err := s.catalog.GetTrack(ctx, id)
		if err != nil {
			continue
		}
		entries = append(entries, TrackEntry{Track: t, Annotation: annPtr(trackAnn, id)})
	}
	return PlayQueueResult{Queue: q, Entries: entries}, nil
}

// Save persists the user's play queue, stamped with the current time.
func (s *PlayQueueService) Save(ctx context.Context, userID, current string, positionMs int64, changedBy string, trackIDs []string) error {
	return s.playQueues.Save(ctx, models.PlayQueue{
		UserID:     userID,
		Current:    current,
		PositionMs: positionMs,
		ChangedBy:  changedBy,
		ChangedAt:  time.Now(),
		TrackIDs:   trackIDs,
	})
}
