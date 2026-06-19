package immerle

import (
	"net/http"
	"strconv"
	"time"
)

// wrappedEnabled reports whether the year-in-review feature is on (defaults to
// on when settings are unavailable, e.g. in tests).
func (h *Handler) wrappedEnabled() bool {
	return h.Settings == nil || h.Settings.WrappedEnabled()
}

// handleWrapped returns the caller's year-in-review for ?year= (default: the
// current UTC year).
//
// @Summary      Year-in-review ("Wrapped")
// @Description  Returns the caller's listening stats for a calendar year: totals, top tracks/artists/genres and a per-month histogram. 404 when the feature is disabled.
// @Tags         wrapped
// @Security     BearerAuth
// @Produce      json
// @Param        year  query  int  false  "Calendar year (default: current)"
// @Success      200  {object}  WrappedDTO
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /wrapped [get]
func (h *Handler) handleWrapped(w http.ResponseWriter, r *http.Request) {
	if !h.wrappedEnabled() || h.Wrapped == nil {
		writeError(w, http.StatusNotFound, "disabled", "wrapped is disabled")
		return
	}
	year := time.Now().UTC().Year()
	if q := r.URL.Query().Get("year"); q != "" {
		if y, err := strconv.Atoi(q); err == nil && y >= 1970 && y <= 9999 {
			year = y
		}
	}
	data, err := h.Wrapped.Wrapped(r.Context(), userFrom(r.Context()).ID, year)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, data)
}

// wrappedStatus is the admin view of the feature toggle.
func (h *Handler) wrappedStatus() map[string]any {
	return map[string]any{"enabled": h.wrappedEnabled()}
}

// handleWrappedAdmin reports whether the feature is enabled.
//
// @Summary      Get the Wrapped feature state
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string]bool
// @Router       /admin/wrapped [get]
func (h *Handler) handleWrappedAdmin(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	writeResource(w, http.StatusOK, h.wrappedStatus())
}

// handleWrappedUpdate turns the year-in-review feature on or off (hot).
//
// @Summary      Toggle the Wrapped feature
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  toggleRequest  true  "Enable or disable"
// @Success      200  {object}  map[string]bool
// @Failure      400  {object}  errorResponse
// @Router       /admin/wrapped [put]
func (h *Handler) handleWrappedUpdate(w http.ResponseWriter, r *http.Request) {
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
	next.Wrapped.Enabled = *req.Enabled
	if _, _, err := h.Settings.Update(next); err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("wrapped toggled", "enabled", *req.Enabled, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, h.wrappedStatus())
}

// toggleRequest is a generic admin on/off body, shared by feature toggles.
type toggleRequest struct {
	Enabled *bool `json:"enabled"`
}
