package immerle

import (
	"encoding/json"
	"net/http"
)

// handleSettings returns the DB-backed runtime settings.
//
// @Summary      Get runtime settings
// @Description  Admin only. Returns the current runtime settings (provider behaviour, artist avatars, scan cadence, federation) plus whether a restart is pending.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  SettingsDTO
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/settings [get]
func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings unavailable")
		return
	}
	writeResource(w, http.StatusOK, settingsBody(h.Settings.Get(), h.Settings.PendingRestart()))
}

// handleSettingsUpdate applies a partial update to the runtime settings.
//
// @Summary      Update runtime settings
// @Description  Admin only. Partial update (send a JSON body with the fields to change; omitted fields keep their current value). Provider behaviour and the scan interval apply immediately (hot reload); avatars, the scan watcher and federation only take effect after a restart — the response sets restartRequired=true and lists the pending fields.
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  RuntimeSettingsDTO  true  "Settings fields to change (partial)"
// @Success      200  {object}  SettingsDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/settings [patch]
func (h *Handler) handleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings unavailable")
		return
	}
	// Decode the body over the current values → partial update.
	next := h.Settings.Get()
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
		writeErrorParams(w, http.StatusBadRequest, "invalid_body", "invalid settings JSON: "+err.Error(), map[string]any{"detail": err.Error()})
		return
	}
	saved, pending, err := h.Settings.Update(next)
	if err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("runtime settings updated", "restartRequired", len(pending) > 0, "pending", pending, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, settingsBody(saved, pending))
}

// handleFederationRegister registers this instance with the hub.
//
// @Summary      Register with the hub
// @Description  Admin only. Claims the configured hub user id (federation.userId) for this instance and persists the hub-assigned instance id (a sqids by default). The full HTTP exchange runs server-side. Returns the refreshed runtime settings.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  SettingsDTO
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation/register [post]
func (h *Handler) handleFederationRegister(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Federation == nil || h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	if err := h.Federation.Register(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "register_failed", err.Error())
		return
	}
	// Register persists the assigned instance id; return the fresh settings.
	writeResource(w, http.StatusOK, settingsBody(h.Settings.Get(), h.Settings.PendingRestart()))
}

func settingsBody(settings any, pending []string) map[string]any {
	if pending == nil {
		pending = []string{}
	}
	return map[string]any{
		"settings":        settings,
		"restartRequired": len(pending) > 0,
		"pendingRestart":  pending,
	}
}
