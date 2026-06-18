package immerle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/immerle/immerle/internal/models"
)

// jamMember reports whether userID is the host or a current participant of the
// session. Used to gate read access to session state and the SSE event stream.
func (h *Handler) jamMember(ctx context.Context, session models.JamSession, userID string) bool {
	if session.HostID == userID {
		return true
	}
	participants, _ := h.Jam.Participants(ctx, session.ID)
	for _, p := range participants {
		if p.UserID == userID {
			return true
		}
	}
	return false
}

// jamView is the wire representation of a session plus its participants.
func (h *Handler) jamView(ctx context.Context, session models.JamSession) map[string]any {
	participants, _ := h.Jam.Participants(ctx, session.ID)
	return map[string]any{"session": session, "participants": participants}
}

// createJamRequest is the body for POST /jam.
type createJamRequest struct {
	Name     string   `json:"name"`
	TrackIDs []string `json:"trackIds"`
}

// handleJamCreate starts a synchronized listening session hosted by the caller.
//
// @Summary      Create a Jam session
// @Tags         jam
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  createJamRequest  true  "Session name and initial track ids"
// @Success      201  {object}  JamDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /jam [post]
func (h *Handler) handleJamCreate(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	var req createJamRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	session, err := h.Jam.Create(r.Context(), user.ID, req.Name, req.TrackIDs)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusCreated, h.jamView(r.Context(), session))
}

// handleJamJoin adds the caller as a participant of a session.
//
// @Summary      Join a Jam session
// @Tags         jam
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Jam session id"
// @Success      201  {object}  JamDTO
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /jam/{id}/participants [post]
func (h *Handler) handleJamJoin(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := pathParam(r, "id")
	if _, err := h.Jam.Get(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	if err := h.Jam.Join(r.Context(), id, user.ID); err != nil {
		writeInternal(w, err)
		return
	}
	session, _ := h.Jam.Get(r.Context(), id)
	writeResource(w, http.StatusCreated, h.jamView(r.Context(), session))
}

// handleJamLeave removes the caller from a session.
//
// @Summary      Leave a Jam session
// @Tags         jam
// @Security     BearerAuth
// @Param        id  path  string  true  "Jam session id"
// @Success      204  "left"
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /jam/{id}/participants/me [delete]
func (h *Handler) handleJamLeave(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if err := h.Jam.Leave(r.Context(), pathParam(r, "id"), user.ID); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// handleJamState returns the current session state to a member.
//
// @Summary      Get Jam session state
// @Tags         jam
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Jam session id"
// @Success      200  {object}  JamDTO
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /jam/{id} [get]
func (h *Handler) handleJamState(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := pathParam(r, "id")
	session, err := h.Jam.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	if !h.jamMember(r.Context(), session, user.ID) {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	writeResource(w, http.StatusOK, h.jamView(r.Context(), session))
}

// updateJamRequest is a partial playback update (host only). Pointer fields
// distinguish "omitted" from a zero value.
type updateJamRequest struct {
	CurrentTrackID *string   `json:"currentTrackId"`
	Position       *int64    `json:"position"`
	State          *string   `json:"state"`
	TrackIDs       *[]string `json:"trackIds"`
}

// handleJamUpdate updates shared playback. Only the host may drive playback.
//
// @Summary      Update Jam playback (host only)
// @Description  Host-only. Updates the shared track/position/state (partial) and broadcasts it to participants over SSE.
// @Tags         jam
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  string            true  "Jam session id"
// @Param        body  body  updateJamRequest  true  "Playback fields to change"
// @Success      200  {object}  JamDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /jam/{id} [patch]
func (h *Handler) handleJamUpdate(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := pathParam(r, "id")
	session, err := h.Jam.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	if session.HostID != user.ID {
		writeError(w, http.StatusForbidden, "forbidden", "only the host can control playback")
		return
	}

	var req updateJamRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	current := session.CurrentTrackID
	if req.CurrentTrackID != nil {
		current = *req.CurrentTrackID
	}
	position := session.PositionMs
	if req.Position != nil {
		position = *req.Position
	}
	state := session.State
	if req.State != nil {
		state = *req.State
	}
	trackIDs := session.TrackIDs
	if req.TrackIDs != nil {
		trackIDs = *req.TrackIDs
	}

	if err := h.Jam.UpdatePlayback(r.Context(), id, current, position, state, trackIDs); err != nil {
		writeInternal(w, err)
		return
	}
	updated, _ := h.Jam.Get(r.Context(), id)
	writeResource(w, http.StatusOK, h.jamView(r.Context(), updated))
}

// handleJamEvents streams jam state changes to a participant over Server-Sent
// Events, keeping clients synchronized to the host's track and position.
//
// @Summary      Stream Jam events (SSE)
// @Description  Server-Sent Events stream. Emits the current state immediately, then a "state"/"participants"/"closed" event on every change.
// @Tags         jam
// @Security     BearerAuth
// @Produce      text/event-stream
// @Param        id  path  string  true  "Jam session id"
// @Success      200  {string}  string  "SSE stream"
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /jam/{id}/events [get]
func (h *Handler) handleJamEvents(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := pathParam(r, "id")
	session, err := h.Jam.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	if !h.jamMember(r.Context(), session, user.ID) {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeInternal(w, fmt.Errorf("streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsubscribe := h.Jam.Subscribe(id)
	defer unsubscribe()

	// Bound each write so a stalled/slow client connection errors out instead of
	// leaking this goroutine and its subscription forever.
	rc := http.NewResponseController(w)
	setDeadline := func() { _ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second)) }

	// Send the current snapshot immediately so a late joiner is in sync.
	participants, _ := h.Jam.Participants(r.Context(), id)
	setDeadline()
	writeEvent(w, flusher, session, participants)

	// Keep-alive so idle connections (and dead peers behind a proxy) are detected.
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			setDeadline()
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, _ := json.Marshal(ev)
			setDeadline()
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, flusher http.Flusher, session models.JamSession, participants []models.JamParticipant) {
	payload, _ := json.Marshal(map[string]any{
		"type":         "state",
		"session":      session,
		"participants": participants,
	})
	fmt.Fprintf(w, "event: state\ndata: %s\n\n", payload)
	flusher.Flush()
}
