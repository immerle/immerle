package core

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/persistence"
)

// JamEvent is broadcast to participants when a jam's state changes.
type JamEvent struct {
	Type         string                  `json:"type"` // "state", "participants", "closed"
	Session      models.JamSession       `json:"session"`
	Participants []models.JamParticipant `json:"participants,omitempty"`
	At           time.Time               `json:"at"`
}

// jamHub fans out jam events to in-process subscribers (SSE/WebSocket clients).
type jamHub struct {
	mu   sync.Mutex
	subs map[string]map[chan JamEvent]struct{}
}

func newJamHub() *jamHub {
	return &jamHub{subs: map[string]map[chan JamEvent]struct{}{}}
}

func (h *jamHub) subscribe(sessionID string) chan JamEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan JamEvent, 16)
	if h.subs[sessionID] == nil {
		h.subs[sessionID] = map[chan JamEvent]struct{}{}
	}
	h.subs[sessionID][ch] = struct{}{}
	return ch
}

func (h *jamHub) unsubscribe(sessionID string, ch chan JamEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subs[sessionID]; ok {
		delete(set, ch)
		close(ch)
		if len(set) == 0 {
			delete(h.subs, sessionID)
		}
	}
}

func (h *jamHub) broadcast(sessionID string, ev JamEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[sessionID] {
		select {
		case ch <- ev:
		default: // drop for slow consumers; they get the next snapshot
		}
	}
}

// JamService manages synchronized listening sessions.
type JamService struct {
	repo *persistence.JamRepo
	hub  *jamHub
}

// NewJamService builds a JamService.
func NewJamService(repo *persistence.JamRepo) *JamService {
	return &JamService{repo: repo, hub: newJamHub()}
}

// Create starts a new jam hosted by hostID.
func (s *JamService) Create(ctx context.Context, hostID, name string, trackIDs []string) (models.JamSession, error) {
	now := time.Now()
	j := models.JamSession{
		ID:        uuid.NewString(),
		HostID:    hostID,
		Name:      name,
		State:     "paused",
		TrackIDs:  trackIDs,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if len(trackIDs) > 0 {
		j.CurrentTrackID = trackIDs[0]
	}
	if err := s.repo.Create(ctx, j); err != nil {
		return models.JamSession{}, err
	}
	if err := s.repo.AddParticipant(ctx, j.ID, hostID); err != nil {
		return models.JamSession{}, err
	}
	return j, nil
}

// Get returns a session.
func (s *JamService) Get(ctx context.Context, id string) (models.JamSession, error) {
	return s.repo.Get(ctx, id)
}

// Participants returns a session's members.
func (s *JamService) Participants(ctx context.Context, id string) ([]models.JamParticipant, error) {
	return s.repo.Participants(ctx, id)
}

// Join adds a user and notifies participants.
func (s *JamService) Join(ctx context.Context, sessionID, userID string) error {
	if err := s.repo.AddParticipant(ctx, sessionID, userID); err != nil {
		return err
	}
	return s.notify(ctx, sessionID, "participants")
}

// Leave removes a user and notifies participants.
func (s *JamService) Leave(ctx context.Context, sessionID, userID string) error {
	if err := s.repo.RemoveParticipant(ctx, sessionID, userID); err != nil {
		return err
	}
	return s.notify(ctx, sessionID, "participants")
}

// UpdatePlayback updates shared playback state (host only enforced by caller)
// and broadcasts it to all participants.
func (s *JamService) UpdatePlayback(ctx context.Context, sessionID, currentTrackID string, positionMs int64, state string, trackIDs []string) error {
	if err := s.repo.UpdatePlayback(ctx, sessionID, currentTrackID, positionMs, state, trackIDs); err != nil {
		return err
	}
	return s.notify(ctx, sessionID, "state")
}

// Close ends a session.
func (s *JamService) Close(ctx context.Context, sessionID string) error {
	_ = s.notify(ctx, sessionID, "closed")
	return s.repo.Delete(ctx, sessionID)
}

// Subscribe returns an event channel and an unsubscribe func for SSE streaming.
func (s *JamService) Subscribe(sessionID string) (<-chan JamEvent, func()) {
	ch := s.hub.subscribe(sessionID)
	return ch, func() { s.hub.unsubscribe(sessionID, ch) }
}

func (s *JamService) notify(ctx context.Context, sessionID, eventType string) error {
	session, err := s.repo.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	participants, _ := s.repo.Participants(ctx, sessionID)
	s.hub.broadcast(sessionID, JamEvent{
		Type:         eventType,
		Session:      session,
		Participants: participants,
		At:           time.Now(),
	})
	return nil
}
