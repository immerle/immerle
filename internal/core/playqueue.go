package core

import (
	"context"
	"sync"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// PlayQueueEvent is broadcast to a user's connected clients whenever their
// play queue changes — a save (new track/position/playing state) or a
// target change (see SetTarget). This is the real-time channel behind
// cross-device sync and remote control (api's GET /play-queue/events).
type PlayQueueEvent struct {
	Queue PlayQueueResult
	At    time.Time
}

// playQueueHub fans out play-queue events to a user's connected clients
// (SSE), mirroring jamHub's per-key pub/sub but keyed by user id.
type playQueueHub struct {
	mu   sync.Mutex
	subs map[string]map[chan PlayQueueEvent]struct{}
}

func newPlayQueueHub() *playQueueHub {
	return &playQueueHub{subs: map[string]map[chan PlayQueueEvent]struct{}{}}
}

func (h *playQueueHub) subscribe(userID string) chan PlayQueueEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan PlayQueueEvent, 16)
	if h.subs[userID] == nil {
		h.subs[userID] = map[chan PlayQueueEvent]struct{}{}
	}
	h.subs[userID][ch] = struct{}{}
	return ch
}

func (h *playQueueHub) unsubscribe(userID string, ch chan PlayQueueEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subs[userID]; ok {
		delete(set, ch)
		close(ch)
		if len(set) == 0 {
			delete(h.subs, userID)
		}
	}
}

func (h *playQueueHub) broadcast(userID string, ev PlayQueueEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[userID] {
		select {
		case ch <- ev:
		default: // drop for slow consumers; they get the next snapshot
		}
	}
}

// PlayQueueService holds the saved play-queue sync logic shared by every
// presentation layer: persisting a user's queue and resolving it back into its
// tracks.
type PlayQueueService struct {
	playQueues  *persistence.PlayQueueRepo
	catalog     *persistence.CatalogRepo
	annotations *persistence.AnnotationRepo
	hub         *playQueueHub
}

// NewPlayQueueService wires the play-queue application service.
func NewPlayQueueService(playQueues *persistence.PlayQueueRepo, catalog *persistence.CatalogRepo, annotations *persistence.AnnotationRepo) *PlayQueueService {
	return &PlayQueueService{playQueues: playQueues, catalog: catalog, annotations: annotations, hub: newPlayQueueHub()}
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

// Save persists the user's play queue, stamped with the current time, and
// notifies subscribers (see Subscribe).
func (s *PlayQueueService) Save(ctx context.Context, userID, current string, positionMs int64, playing bool, changedBy string, trackIDs []string) error {
	if err := s.playQueues.Save(ctx, models.PlayQueue{
		UserID:     userID,
		Current:    current,
		PositionMs: positionMs,
		Playing:    playing,
		ChangedBy:  changedBy,
		ChangedAt:  time.Now(),
		TrackIDs:   trackIDs,
	}); err != nil {
		return err
	}
	s.notify(ctx, userID)
	return nil
}

// SetTarget assigns (deviceID != "") or clears (deviceID == "") the sole
// device that should be actively playing the user's queue, and notifies
// subscribers (see Subscribe).
func (s *PlayQueueService) SetTarget(ctx context.Context, userID, deviceID string) error {
	if err := s.playQueues.SetTarget(ctx, userID, deviceID); err != nil {
		return err
	}
	s.notify(ctx, userID)
	return nil
}

// Subscribe returns an event channel and an unsubscribe func for SSE
// streaming of a user's play-queue changes.
func (s *PlayQueueService) Subscribe(userID string) (<-chan PlayQueueEvent, func()) {
	ch := s.hub.subscribe(userID)
	return ch, func() { s.hub.unsubscribe(userID, ch) }
}

func (s *PlayQueueService) notify(ctx context.Context, userID string) {
	res, err := s.Get(ctx, userID)
	if err != nil {
		return
	}
	s.hub.broadcast(userID, PlayQueueEvent{Queue: res, At: time.Now()})
}
