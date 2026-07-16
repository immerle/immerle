package core

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// JamEvent is broadcast to participants when a jam's state changes.
type JamEvent struct {
	Type         string                  `json:"type"` // "state", "participants", "closed"
	Session      models.JamSession       `json:"session"`
	Participants []models.JamParticipant `json:"participants,omitempty"`
	At           time.Time               `json:"at"`
}

// eventHub fans out per-key events to in-process subscribers (SSE clients).
// Used both for jam-session state (keyed by session id) and per-user Jam
// invite lists (keyed by user id).
type eventHub[T any] struct {
	mu   sync.Mutex
	subs map[string]map[chan T]struct{}
}

func newEventHub[T any]() *eventHub[T] {
	return &eventHub[T]{subs: map[string]map[chan T]struct{}{}}
}

func (h *eventHub[T]) subscribe(key string) chan T {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan T, 16)
	if h.subs[key] == nil {
		h.subs[key] = map[chan T]struct{}{}
	}
	h.subs[key][ch] = struct{}{}
	return ch
}

func (h *eventHub[T]) unsubscribe(key string, ch chan T) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subs[key]; ok {
		delete(set, ch)
		close(ch)
		if len(set) == 0 {
			delete(h.subs, key)
		}
	}
}

func (h *eventHub[T]) broadcast(key string, ev T) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[key] {
		select {
		case ch <- ev:
		default: // drop for slow consumers; they get the next snapshot
		}
	}
}

// JamService manages synchronized listening sessions.
type JamService struct {
	repo      *persistence.JamRepo
	hub       *eventHub[JamEvent]
	inviteHub *eventHub[[]models.JamInvite]
}

// NewJamService builds a JamService.
func NewJamService(repo *persistence.JamRepo) *JamService {
	return &JamService{repo: repo, hub: newEventHub[JamEvent](), inviteHub: newEventHub[[]models.JamInvite]()}
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
	// Joining accepts any pending invite to this session — it stops showing up
	// in the invitee's pending list.
	_ = s.repo.DeleteInvitesForSession(ctx, sessionID, userID)
	s.notifyInvites(ctx, userID)
	return s.notify(ctx, sessionID, "participants")
}

// GetByHost returns the caller's currently-hosted session, if any — the
// header button's "do I already have a Jam running" check.
func (s *JamService) GetByHost(ctx context.Context, hostID string) (models.JamSession, error) {
	return s.repo.GetByHost(ctx, hostID)
}

// Invite invites a user to a session (the handler enforces that the caller is
// the host). Re-inviting just refreshes the invite so it resurfaces.
func (s *JamService) Invite(ctx context.Context, sessionID, inviterID, inviteeID string) error {
	if err := s.repo.CreateInvite(ctx, models.JamInvite{
		ID: uuid.NewString(), SessionID: sessionID, InviterID: inviterID, InviteeID: inviteeID, CreatedAt: time.Now(),
	}); err != nil {
		return err
	}
	s.notifyInvites(ctx, inviteeID)
	return nil
}

// MyInvites returns the pending Jam invites addressed to a user.
func (s *JamService) MyInvites(ctx context.Context, userID string) ([]models.JamInvite, error) {
	return s.repo.ListInvitesForInvitee(ctx, userID)
}

// DismissInvite removes one pending invite, scoped to its invitee.
func (s *JamService) DismissInvite(ctx context.Context, id, inviteeID string) error {
	if err := s.repo.DeleteInviteForInvitee(ctx, id, inviteeID); err != nil {
		return err
	}
	s.notifyInvites(ctx, inviteeID)
	return nil
}

// SubscribeInvites returns an event channel and unsubscribe func for SSE
// streaming of a user's pending Jam invites — a fresh snapshot is pushed
// whenever it changes (invited, dismissed, or consumed by joining).
func (s *JamService) SubscribeInvites(userID string) (<-chan []models.JamInvite, func()) {
	ch := s.inviteHub.subscribe(userID)
	return ch, func() { s.inviteHub.unsubscribe(userID, ch) }
}

// notifyInvites re-fetches and broadcasts a user's current pending invites.
func (s *JamService) notifyInvites(ctx context.Context, userID string) {
	invites, err := s.repo.ListInvitesForInvitee(ctx, userID)
	if err != nil {
		return
	}
	s.inviteHub.broadcast(userID, invites)
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
