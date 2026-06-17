package immerle

import (
	"net/http"
	"strconv"
)

// requireAdmin writes a 403 and returns false if the caller is not an admin.
func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !userFrom(r.Context()).IsAdmin {
		writeJSON(w, http.StatusForbidden, errorBody("admin only"))
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

// handleCleanup reports or changes the provider-download eviction state. The
// state lives in the runtime settings (data/configuration.yaml); toggling it is
// hot.
//
// @Summary      Get or toggle the cleanup sweep
// @Description  Admin only. GET reports the eviction sweep state; POST with enabled=true|false turns the background sweep on or off at runtime (persisted; hot).
// @Tags         admin
// @Produce      json
// @Param        u        query  string  true   "Subsonic username (or use a Bearer token)"
// @Param        p        query  string  false  "Subsonic password"
// @Param        c        query  string  true   "Client name"
// @Param        enabled  query  bool    false  "POST only: enable or disable the sweep"
// @Success      200  {object}  CleanupStatusResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /admin/cleanup [get]
// @Router       /admin/cleanup [post]
func (h *Handler) handleCleanup(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Settings == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorBody("cleanup not available"))
		return
	}
	if r.Method == http.MethodPost {
		enabled, err := strconv.ParseBool(r.Form.Get("enabled"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody("enabled must be true or false"))
			return
		}
		next := h.Settings.Get()
		next.Cleanup.Enabled = enabled
		if _, _, err := h.Settings.Update(next); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
			return
		}
		h.Logger.Info("cleanup sweep toggled", "enabled", enabled, "by", userFrom(r.Context()).Username)
	}
	writeJSON(w, http.StatusOK, okBody(h.cleanupStatus()))
}

// handleCleanupRun triggers an immediate eviction sweep, regardless of whether
// the background sweep is enabled.
//
// @Summary      Run the cleanup sweep now
// @Description  Admin only. Runs one eviction pass immediately and returns how many provider downloads were removed. Works even when the background sweep is disabled.
// @Tags         admin
// @Produce      json
// @Param        u  query  string  true   "Subsonic username (or use a Bearer token)"
// @Param        p  query  string  false  "Subsonic password"
// @Param        c  query  string  true   "Client name"
// @Success      200  {object}  CleanupRunResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /admin/cleanup/run [post]
func (h *Handler) handleCleanupRun(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Cleanup == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorBody("cleanup not available"))
		return
	}
	removed, err := h.Cleanup.Sweep(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	h.Logger.Info("cleanup sweep run on demand", "removed", removed, "by", userFrom(r.Context()).Username)
	writeJSON(w, http.StatusOK, okBody(map[string]any{"removed": removed}))
}
