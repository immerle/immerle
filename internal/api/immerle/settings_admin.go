package immerle

import (
	"encoding/json"
	"net/http"
)

// handleSettings reads (GET) or updates (POST) the DB-backed runtime settings.
//
// @Summary      Get or update runtime settings
// @Description  Admin only. GET returns the current runtime settings (provider behaviour, artist avatars, scan cadence, federation) plus whether a restart is pending. POST applies a partial update (send a JSON body with the fields to change; omitted fields keep their current value). Provider behaviour and the scan interval apply immediately (hot reload); avatars, the scan watcher and federation only take effect after a restart — when one of those changes, the response sets restartRequired=true and lists the pending fields so the UI can prompt for a restart.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        u  query  string  true   "Subsonic username (or Bearer token)"
// @Param        p  query  string  false  "Subsonic password"
// @Param        c  query  string  true   "Client name"
// @Param        body  body  RuntimeSettingsDTO  false  "POST: settings fields to change (partial)"
// @Success      200  {object}  SettingsResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /admin/settings [get]
// @Router       /admin/settings [post]
func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Settings == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorBody("settings unavailable"))
		return
	}

	if r.Method == http.MethodPost {
		// Decode the body over the current values → partial update.
		next := h.Settings.Get()
		if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody("invalid settings JSON: "+err.Error()))
			return
		}
		saved, pending, err := h.Settings.Update(next)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
			return
		}
		h.Logger.Info("runtime settings updated", "restartRequired", len(pending) > 0, "pending", pending, "by", userFrom(r.Context()).Username)
		writeJSON(w, http.StatusOK, settingsBody(saved, pending))
		return
	}

	writeJSON(w, http.StatusOK, settingsBody(h.Settings.Get(), h.Settings.PendingRestart()))
}

func settingsBody(settings any, pending []string) map[string]any {
	if pending == nil {
		pending = []string{}
	}
	return okBody(map[string]any{
		"settings":        settings,
		"restartRequired": len(pending) > 0,
		"pendingRestart":  pending,
	})
}
