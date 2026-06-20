package immerle

import "net/http"

// offlineEnabled reports whether offline downloads are on (defaults to on when
// settings are unavailable, e.g. in tests).
func (h *Handler) offlineEnabled() bool {
	return h.Settings == nil || h.Settings.OfflineEnabled()
}

// offlineStatus is the admin view of the feature toggle.
func (h *Handler) offlineStatus() map[string]any {
	return map[string]any{"enabled": h.offlineEnabled()}
}

// handleOfflineAdmin reports whether offline downloads are enabled.
//
// @Summary      Get the offline-downloads feature state
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string]bool
// @Router       /admin/offline [get]
func (h *Handler) handleOfflineAdmin(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	writeResource(w, http.StatusOK, h.offlineStatus())
}

// handleOfflineUpdate turns offline downloads on or off (hot).
//
// @Summary      Toggle the offline-downloads feature
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  toggleRequest  true  "Enable or disable"
// @Success      200  {object}  map[string]bool
// @Failure      400  {object}  errorResponse
// @Router       /admin/offline [put]
func (h *Handler) handleOfflineUpdate(w http.ResponseWriter, r *http.Request) {
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
	next.Offline.Enabled = *req.Enabled
	if _, _, err := h.Settings.Update(next); err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("offline downloads toggled", "enabled", *req.Enabled, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, h.offlineStatus())
}
