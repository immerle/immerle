package immerle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/immerle/immerle/internal/models"
)

// @Summary      Create a Jam session
// @Description  Starts a synchronized listening session hosted by the caller.
// @Tags         jam
// @Produce      json
// @Param        u         query  string  true   "Subsonic username"
// @Param        p         query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c         query  string  true   "Client name"
// @Param        name      query  string  false  "Session name"
// @Param        trackIds  query  string  false  "Comma-separated track ids"
// @Success      200  {object}  JamResponse
// @Router       /jam/create [post]
func (h *Handler) handleJamCreate(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	name := r.Form.Get("name")
	var trackIDs []string
	if v := r.Form.Get("trackIds"); v != "" {
		trackIDs = strings.Split(v, ",")
	}
	session, err := h.Jam.Create(r.Context(), user.ID, name, trackIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"session": session}))
}

// @Summary      Join a Jam session
// @Tags         jam
// @Produce      json
// @Param        u          query  string  true   "Subsonic username"
// @Param        p          query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c          query  string  true   "Client name"
// @Param        sessionId  query  string  true   "Jam session id"
// @Success      200  {object}  JamResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /jam/join [post]
func (h *Handler) handleJamJoin(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := r.Form.Get("sessionId")
	if _, err := h.Jam.Get(r.Context(), id); err != nil {
		writeJSON(w, http.StatusNotFound, errorBody("session not found"))
		return
	}
	if err := h.Jam.Join(r.Context(), id, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	session, _ := h.Jam.Get(r.Context(), id)
	writeJSON(w, http.StatusOK, okBody(map[string]any{"session": session}))
}

// @Summary      Leave a Jam session
// @Tags         jam
// @Produce      json
// @Param        u          query  string  true   "Subsonic username"
// @Param        p          query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c          query  string  true   "Client name"
// @Param        sessionId  query  string  true   "Jam session id"
// @Success      200  {object}  OKResponse
// @Router       /jam/leave [post]
func (h *Handler) handleJamLeave(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := r.Form.Get("sessionId")
	if err := h.Jam.Leave(r.Context(), id, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, okBody(nil))
}

// @Summary      Get Jam session state
// @Tags         jam
// @Produce      json
// @Param        u          query  string  true   "Subsonic username"
// @Param        p          query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c          query  string  true   "Client name"
// @Param        sessionId  query  string  true   "Jam session id"
// @Success      200  {object}  JamResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /jam/state [get]
func (h *Handler) handleJamState(w http.ResponseWriter, r *http.Request) {
	id := r.Form.Get("sessionId")
	session, err := h.Jam.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody("session not found"))
		return
	}
	participants, _ := h.Jam.Participants(r.Context(), id)
	writeJSON(w, http.StatusOK, okBody(map[string]any{"session": session, "participants": participants}))
}

// handleJamUpdate updates shared playback. Only the host may drive playback.
//
// @Summary      Update Jam playback (host only)
// @Description  Host-only. Updates the shared track/position/state and broadcasts it to participants over SSE.
// @Tags         jam
// @Produce      json
// @Param        u               query  string  true   "Subsonic username"
// @Param        p               query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c               query  string  true   "Client name"
// @Param        sessionId       query  string  true   "Jam session id"
// @Param        currentTrackId  query  string  false  "Current track id"
// @Param        position        query  int     false  "Playback position in ms"
// @Param        state           query  string  false  "playing or paused"
// @Param        trackIds        query  string  false  "Comma-separated track ids"
// @Success      200  {object}  JamResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /jam/update [post]
func (h *Handler) handleJamUpdate(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := r.Form.Get("sessionId")
	session, err := h.Jam.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody("session not found"))
		return
	}
	if session.HostID != user.ID {
		writeJSON(w, http.StatusForbidden, errorBody("only the host can control playback"))
		return
	}

	current := session.CurrentTrackID
	if v := r.Form.Get("currentTrackId"); v != "" {
		current = v
	}
	position := session.PositionMs
	if v := r.Form.Get("position"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			position = n
		}
	}
	state := session.State
	if v := r.Form.Get("state"); v != "" {
		state = v
	}
	trackIDs := session.TrackIDs
	if v := r.Form.Get("trackIds"); v != "" {
		trackIDs = strings.Split(v, ",")
	}

	if err := h.Jam.UpdatePlayback(r.Context(), id, current, position, state, trackIDs); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	updated, _ := h.Jam.Get(r.Context(), id)
	writeJSON(w, http.StatusOK, okBody(map[string]any{"session": updated}))
}

// handleJamEvents streams jam state changes to a participant over Server-Sent
// Events, keeping clients synchronized to the host's track and position.
//
// @Summary      Stream Jam events (SSE)
// @Description  Server-Sent Events stream. Emits the current state immediately, then a "state"/"participants"/"closed" event on every change.
// @Tags         jam
// @Produce      text/event-stream
// @Param        u          query  string  true   "Subsonic username"
// @Param        p          query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c          query  string  true   "Client name"
// @Param        sessionId  query  string  true   "Jam session id"
// @Success      200  {string}  string  "SSE stream"
// @Failure      404  {object}  ErrorResponse
// @Router       /jam/events [get]
func (h *Handler) handleJamEvents(w http.ResponseWriter, r *http.Request) {
	id := r.Form.Get("sessionId")
	session, err := h.Jam.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody("session not found"))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, errorBody("streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsubscribe := h.Jam.Subscribe(id)
	defer unsubscribe()

	// Send the current snapshot immediately so a late joiner is in sync.
	participants, _ := h.Jam.Participants(r.Context(), id)
	writeEvent(w, flusher, session, participants)

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload)
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
