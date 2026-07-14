package immerle

import (
	"net/http"
	"time"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
)

// hallOfFameEnabled reports whether the Hall of Fame feature is on (defaults
// to on when settings are unavailable, e.g. in tests).
func (h *Handler) hallOfFameEnabled() bool {
	return h.Settings == nil || h.Settings.HallOfFameEnabled()
}

// hallOfFameStatus is the admin view of the feature toggle.
func (h *Handler) hallOfFameStatus() map[string]any {
	return map[string]any{"enabled": h.hallOfFameEnabled()}
}

// handleHallOfFameAdmin reports whether the Hall of Fame feature is enabled.
//
// @Summary  Get the Hall of Fame feature state
// @Tags     admin
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  map[string]bool
// @Router   /admin/hall-of-fame [get]
func (h *Handler) handleHallOfFameAdmin(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	writeResource(w, http.StatusOK, h.hallOfFameStatus())
}

// handleHallOfFameToggle turns the Hall of Fame feature on or off (hot).
//
// @Summary  Toggle the Hall of Fame feature
// @Tags     admin
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body  body  toggleRequest  true  "Enable or disable"
// @Success  200  {object}  map[string]bool
// @Failure  400  {object}  errorResponse
// @Router   /admin/hall-of-fame [put]
func (h *Handler) handleHallOfFameToggle(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings not available")
		return
	}
	var req toggleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "validation", "enabled is required")
		return
	}
	next := h.Settings.Get()
	next.HallOfFame.Enabled = *req.Enabled
	if _, _, err := h.Settings.Update(next); err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("hall of fame toggled", "enabled", *req.Enabled, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, h.hallOfFameStatus())
}

// hallOfFameView is the REST representation of a Hall of Fame.
type hallOfFameView struct {
	Tracks    []songView `json:"tracks"`
	CreatedAt time.Time  `json:"createdAt"`
	ChangedAt time.Time  `json:"changedAt"`
}

func hallOfFameEntriesToSongViews(entries []models.HallOfFameEntry) []songView {
	out := make([]songView, 0, len(entries))
	for _, e := range entries {
		v := toSongView(e.Track)
		v.Comment = e.Comment
		out = append(out, v)
	}
	return out
}

func detailToHallOfFameView(d core.HallOfFameDetail) hallOfFameView {
	return hallOfFameView{
		Tracks:    hallOfFameEntriesToSongViews(d.Entries),
		CreatedAt: d.HallOfFame.CreatedAt,
		ChangedAt: d.HallOfFame.UpdatedAt,
	}
}

// handleGetHallOfFame returns the caller's Hall of Fame, auto-creating it on
// first access. 404 when the feature is disabled.
//
// @Summary  Get (or create) the caller's Hall of Fame
// @Tags     playlists
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  hallOfFameView
// @Failure  401  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /hall-of-fame [get]
func (h *Handler) handleGetHallOfFame(w http.ResponseWriter, r *http.Request) {
	if !h.hallOfFameEnabled() {
		writeError(w, http.StatusNotFound, "disabled", "hall of fame is disabled")
		return
	}
	d, err := h.hallOfFameSvc.Get(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, detailToHallOfFameView(d))
}

// hallOfFameOrderRequest is the body for PUT /hall-of-fame/tracks.
type hallOfFameOrderRequest struct {
	IDs []string `json:"ids"`
}

// handleSetHallOfFameOrder replaces the caller's full ranked track list
// (reorder, add and remove all go through this one call).
//
// @Summary  Set the Hall of Fame's track order
// @Tags     playlists
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body  body  hallOfFameOrderRequest  true  "Track ids, in rank order"
// @Success  200  {object}  hallOfFameView
// @Failure  401  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /hall-of-fame/tracks [put]
func (h *Handler) handleSetHallOfFameOrder(w http.ResponseWriter, r *http.Request) {
	if !h.hallOfFameEnabled() {
		writeError(w, http.StatusNotFound, "disabled", "hall of fame is disabled")
		return
	}
	var req hallOfFameOrderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	userID := userFrom(r.Context()).ID
	if err := h.hallOfFameSvc.SetOrder(r.Context(), userID, req.IDs); err != nil {
		writeServiceError(w, err)
		return
	}
	d, err := h.hallOfFameSvc.Get(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, detailToHallOfFameView(d))
}

// hallOfFameAddRequest is the body for POST /hall-of-fame/tracks.
type hallOfFameAddRequest struct {
	ID string `json:"id"`
}

// handleAddHallOfFameTrack appends one track to the caller's Hall of Fame (a
// no-op if it's already there) — the quick "Add to Hall of Fame" track-menu action.
//
// @Summary  Add a track to the Hall of Fame
// @Tags     playlists
// @Security BearerAuth
// @Accept   json
// @Param    body  body  hallOfFameAddRequest  true  "Track id"
// @Success  204  "No Content"
// @Failure  400  {object}  errorResponse
// @Failure  401  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /hall-of-fame/tracks [post]
func (h *Handler) handleAddHallOfFameTrack(w http.ResponseWriter, r *http.Request) {
	if !h.hallOfFameEnabled() {
		writeError(w, http.StatusNotFound, "disabled", "hall of fame is disabled")
		return
	}
	var req hallOfFameAddRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ID == "" {
		writeValidation(w, []fieldError{{Field: "id", Message: "id is required"}})
		return
	}
	if err := h.hallOfFameSvc.Add(r.Context(), userFrom(r.Context()).ID, req.ID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// hallOfFameNoteRequest is the body for PATCH /hall-of-fame/tracks/{trackId}/note.
type hallOfFameNoteRequest struct {
	Comment string `json:"comment"`
}

// handleSetHallOfFameNote sets (or, given an empty comment, clears) a
// personal nostalgia note on one of the caller's ranked tracks — e.g.
// "listened to this in college".
//
// @Summary  Set a Hall of Fame track's note
// @Tags     playlists
// @Security BearerAuth
// @Accept   json
// @Param    trackId  path  string                 true  "Track id"
// @Param    body     body  hallOfFameNoteRequest  true  "Note"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /hall-of-fame/tracks/{trackId}/note [patch]
func (h *Handler) handleSetHallOfFameNote(w http.ResponseWriter, r *http.Request) {
	if !h.hallOfFameEnabled() {
		writeError(w, http.StatusNotFound, "disabled", "hall of fame is disabled")
		return
	}
	var req hallOfFameNoteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.hallOfFameSvc.SetNote(r.Context(), userFrom(r.Context()).ID, pathParam(r, "trackId"), req.Comment); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}
