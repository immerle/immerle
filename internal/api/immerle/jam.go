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
func (h *Handler) jamMember(ctx context.Context, session models.JamSession, userID string) (bool, error) {
	if session.HostID == userID {
		return true, nil
	}
	participants, err := h.Jam.Participants(ctx, session.ID)
	if err != nil {
		return false, err
	}
	for _, p := range participants {
		if p.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}

// requireJamMember answers 404 (not 403, to not reveal the session) when the
// user is not a member, and 500 when membership cannot be checked.
func (h *Handler) requireJamMember(w http.ResponseWriter, r *http.Request, session models.JamSession, userID string) bool {
	member, err := h.jamMember(r.Context(), session, userID)
	if err != nil {
		writeInternal(w, err)
		return false
	}
	if !member {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return false
	}
	return true
}

// jamView is the wire representation of a session plus its participants. The
// models types are what the SSE events carry too, so REST and SSE never drift.
type jamView struct {
	Session      models.JamSession       `json:"session"`
	Participants []models.JamParticipant `json:"participants" validate:"required"`
}

// writeJam answers with the session and its participants.
func (h *Handler) writeJam(w http.ResponseWriter, r *http.Request, status int, session models.JamSession) {
	participants, err := h.Jam.Participants(r.Context(), session.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	if participants == nil {
		participants = []models.JamParticipant{}
	}
	writeResource(w, status, jamView{Session: session, Participants: participants})
}

// writeJamAfterChange re-reads the session after a successful write and
// answers with it: 404 if it ended meanwhile (e.g. closed by the host from
// another tab), 500 if it cannot be read.
func (h *Handler) writeJamAfterChange(w http.ResponseWriter, r *http.Request, status int, id string) {
	session, err := h.Jam.Get(r.Context(), id)
	if err != nil {
		writeLookupError(w, err, "session ended")
		return
	}
	h.writeJam(w, r, status, session)
}

// jamInviteListView is the caller's pending invites (REST and the SSE
// "invites" event).
type jamInviteListView struct {
	Invites []models.JamInvite `json:"invites" validate:"required"`
}

func jamInviteList(invites []models.JamInvite) jamInviteListView {
	if invites == nil {
		invites = []models.JamInvite{}
	}
	return jamInviteListView{Invites: invites}
}

// createJamRequest is the body for POST /jam.
type createJamRequest struct {
	Name     string   `json:"name" validate:"optional"`
	TrackIDs []string `json:"trackIds,omitempty" validate:"optional"`
}

// handleJamCreate starts a synchronized listening session hosted by the caller.
//
// @Summary      Create a Jam session
// @Tags         jam
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  createJamRequest  true  "Session name and initial track ids"
// @Success      201  {object}  jamView
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
	h.writeJam(w, r, http.StatusCreated, session)
}

// handleJamMine returns the session the caller is currently hosting, if any —
// 404 when they aren't hosting one. Backs the header button's create-vs-invite
// state (the in-memory client-side jam store isn't persisted, so it can't
// answer this reliably after a reload).
//
// @Summary      Get the caller's hosted Jam session
// @Tags         jam
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  jamView
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /jam/mine [get]
func (h *Handler) handleJamMine(w http.ResponseWriter, r *http.Request) {
	session, err := h.Jam.GetByHost(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeLookupError(w, err, "not hosting a session")
		return
	}
	h.writeJam(w, r, http.StatusOK, session)
}

// handleJamJoin adds the caller as a participant of a session.
//
// @Summary      Join a Jam session
// @Tags         jam
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Jam session id"
// @Success      201  {object}  jamView
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /jam/{id}/participants [post]
func (h *Handler) handleJamJoin(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := pathParam(r, "id")
	if _, err := h.Jam.Get(r.Context(), id); err != nil {
		writeLookupError(w, err, "session not found")
		return
	}
	if err := h.Jam.Join(r.Context(), id, user.ID); err != nil {
		writeInternal(w, err)
		return
	}
	h.writeJamAfterChange(w, r, http.StatusCreated, id)
}

// jamInviteRequest is the body for POST /jam/{id}/invites.
type jamInviteRequest struct {
	Username string `json:"username" validate:"optional"`
}

// handleJamInvite invites a user to the caller's session. Host only.
//
// @Summary      Invite a user to a Jam session
// @Description  Invites a user to the session. Host only; re-inviting just refreshes the invite.
// @Tags         jam
// @Security     BearerAuth
// @Accept       json
// @Param        id    path  string             true  "Jam session id"
// @Param        body  body  jamInviteRequest  true  "Invitee username"
// @Success      204  "invited"
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /jam/{id}/invites [post]
func (h *Handler) handleJamInvite(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := pathParam(r, "id")
	session, err := h.Jam.Get(r.Context(), id)
	if err != nil {
		writeLookupError(w, err, "session not found")
		return
	}
	if session.HostID != user.ID {
		writeError(w, http.StatusForbidden, "forbidden", "only the host can invite")
		return
	}
	var req jamInviteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	invitee, err := h.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		writeLookupError(w, err, "user not found")
		return
	}
	if err := h.Jam.Invite(r.Context(), id, user.ID, invitee.ID); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// handleJamInvitesMine lists the pending Jam invites addressed to the caller.
//
// @Summary      List the caller's pending Jam invites
// @Tags         jam
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  jamInviteListView
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /jam/invites/mine [get]
func (h *Handler) handleJamInvitesMine(w http.ResponseWriter, r *http.Request) {
	invites, err := h.Jam.MyInvites(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, jamInviteList(invites))
}

// handleJamInviteDismiss removes one pending invite — the invitee declining
// it (accepting is just joining the session normally, which also clears it).
//
// @Summary      Dismiss a pending Jam invite
// @Tags         jam
// @Security     BearerAuth
// @Param        id  path  string  true  "Invite id"
// @Success      204  "dismissed"
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /jam/invites/{id} [delete]
func (h *Handler) handleJamInviteDismiss(w http.ResponseWriter, r *http.Request) {
	if err := h.Jam.DismissInvite(r.Context(), pathParam(r, "id"), userFrom(r.Context()).ID); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// writeInvitesEvent writes the caller's pending Jam invites as an "invites"
// SSE event. Shared by handleStreamPlayQueue, which carries this alongside
// play-queue events on the caller's one always-open connection rather than a
// dedicated stream — see its doc comment for why.
func writeInvitesEvent(w http.ResponseWriter, flusher http.Flusher, invites []models.JamInvite) {
	payload, _ := json.Marshal(jamInviteList(invites))
	fmt.Fprintf(w, "event: invites\ndata: %s\n\n", payload)
	flusher.Flush()
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

// handleJamDelete ends a session. Only the host may end it; doing so removes the
// session (and its participants).
//
// @Summary      End a Jam session
// @Description  Ends and removes the session. Host only.
// @Tags         jam
// @Security     BearerAuth
// @Param        id  path  string  true  "Jam session id"
// @Success      204  "ended"
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /jam/{id} [delete]
func (h *Handler) handleJamDelete(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	session, err := h.Jam.Get(r.Context(), id)
	if err != nil {
		writeLookupError(w, err, "session not found")
		return
	}
	if session.HostID != userFrom(r.Context()).ID {
		writeError(w, http.StatusForbidden, "forbidden", "only the host can end the session")
		return
	}
	if err := h.Jam.Close(r.Context(), id); err != nil {
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
// @Success      200  {object}  jamView
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /jam/{id} [get]
func (h *Handler) handleJamState(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := pathParam(r, "id")
	session, err := h.Jam.Get(r.Context(), id)
	if err != nil {
		writeLookupError(w, err, "session not found")
		return
	}
	if !h.requireJamMember(w, r, session, user.ID) {
		return
	}
	h.writeJam(w, r, http.StatusOK, session)
}

// updateJamRequest is a partial playback update (host only). Pointer fields
// distinguish "omitted" from a zero value.
type updateJamRequest struct {
	CurrentTrackID *string   `json:"currentTrackId,omitempty" validate:"optional"`
	Position       *int64    `json:"position,omitempty" validate:"optional"`
	State          *string   `json:"state,omitempty" validate:"optional"`
	TrackIDs       *[]string `json:"trackIds,omitempty" validate:"optional"`
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
// @Success      200  {object}  jamView
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
		writeLookupError(w, err, "session not found")
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
	h.writeJamAfterChange(w, r, http.StatusOK, id)
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
		writeLookupError(w, err, "session not found")
		return
	}
	if !h.requireJamMember(w, r, session, user.ID) {
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
	participants, err := h.Jam.Participants(r.Context(), id)
	if err != nil {
		h.Logger.Warn("jam snapshot failed", "session", id, "error", err)
		return
	}
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
