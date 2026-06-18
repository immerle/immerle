package immerle

import (
	"net/http"
)

// requireAdmin writes a 403 and returns false if the caller is not an admin.
func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !userFrom(r.Context()).IsAdmin {
		writeError(w, http.StatusForbidden, "forbidden", "admin only")
		return false
	}
	return true
}

// cleanupStatus builds the current eviction sweep state from the runtime config.
func (h *Handler) cleanupStatus() map[string]any {
	c := h.Settings.Get().Cleanup
	return map[string]any{
		"enabled":         c.Enabled,
		"maxAgeSeconds":   c.MaxAgeSeconds,
		"intervalSeconds": c.IntervalSeconds,
	}
}

// handleCleanup reports the provider-download eviction sweep state.
//
// @Summary      Get the cleanup sweep state
// @Description  Admin only. Reports the eviction sweep state.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  CleanupStatusDTO
// @Failure      401  {object}  apiError
// @Failure      403  {object}  apiError
// @Failure      503  {object}  apiError
// @Router       /admin/cleanup [get]
func (h *Handler) handleCleanup(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "cleanup not available")
		return
	}
	writeResource(w, http.StatusOK, h.cleanupStatus())
}

// cleanupUpdateRequest is the body for PUT /admin/cleanup.
type cleanupUpdateRequest struct {
	Enabled *bool `json:"enabled"`
}

// handleCleanupUpdate turns the background sweep on or off at runtime.
//
// @Summary      Toggle the cleanup sweep
// @Description  Admin only. Enables or disables the background eviction sweep at runtime (persisted; hot).
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  cleanupUpdateRequest  true  "Enable or disable the sweep"
// @Success      200  {object}  CleanupStatusDTO
// @Failure      400  {object}  apiError
// @Failure      401  {object}  apiError
// @Failure      403  {object}  apiError
// @Failure      500  {object}  apiError
// @Failure      503  {object}  apiError
// @Router       /admin/cleanup [put]
func (h *Handler) handleCleanupUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "cleanup not available")
		return
	}
	var req cleanupUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "validation", "enabled is required")
		return
	}
	next := h.Settings.Get()
	next.Cleanup.Enabled = *req.Enabled
	if _, _, err := h.Settings.Update(next); err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("cleanup sweep toggled", "enabled", *req.Enabled, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, h.cleanupStatus())
}

// handleCleanupRun triggers an immediate eviction sweep, regardless of whether
// the background sweep is enabled.
//
// @Summary      Run the cleanup sweep now
// @Description  Admin only. Runs one eviction pass immediately and returns how many provider downloads were removed. Works even when the background sweep is disabled.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      201  {object}  CleanupRunDTO
// @Failure      401  {object}  apiError
// @Failure      403  {object}  apiError
// @Failure      500  {object}  apiError
// @Failure      503  {object}  apiError
// @Router       /admin/cleanup/runs [post]
func (h *Handler) handleCleanupRun(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Cleanup == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "cleanup not available")
		return
	}
	removed, err := h.Cleanup.Sweep(r.Context())
	if err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("cleanup sweep run on demand", "removed", removed, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusCreated, map[string]any{"removed": removed})
}
