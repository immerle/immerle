package immerle

import (
	"encoding/json"
	"net/http"

	"github.com/immerle/immerle/internal/models"
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
	writeResource(w, http.StatusOK, settingsBody(redactSettings(h.Settings.Get()), h.Settings.PendingRestart()))
}

// redactSettings clears secrets that must never leave the server (the hub
// private key) before serializing settings to an admin client.
func redactSettings(rs models.RuntimeSettings) models.RuntimeSettings {
	rs.Federation.PrivateKey = ""
	return rs
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
	current := h.Settings.Get()
	next := current
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
		writeErrorParams(w, http.StatusBadRequest, "invalid_body", "invalid settings JSON: "+err.Error(), map[string]any{"detail": err.Error()})
		return
	}
	// Hub-managed federation identity is never settable via this generic endpoint:
	// the instance UUID, sqid and private key come from the hub (bootstrap /
	// /admin/federation), so restore them from the current values regardless of
	// what the client sent.
	next.Federation.InstanceID = current.Federation.InstanceID
	next.Federation.Sqid = current.Federation.Sqid
	next.Federation.InstanceName = current.Federation.InstanceName
	next.Federation.PrivateKey = current.Federation.PrivateKey
	saved, pending, err := h.Settings.Update(next)
	if err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("runtime settings updated", "restartRequired", len(pending) > 0, "pending", pending, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, settingsBody(redactSettings(saved), pending))
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
	// Register persists the assigned id + private key; return the fresh settings.
	writeResource(w, http.StatusOK, settingsBody(redactSettings(h.Settings.Get()), h.Settings.PendingRestart()))
}

// handleFederationUpdate pushes a name / sqid change to the hub.
//
// @Summary      Update this instance on the hub
// @Description  Admin only. Pushes the instance name and sqid (the editable, unique hub handle) to the hub, which validates sqid uniqueness, then persists the hub-canonical values. The HTTP exchange runs server-side. Returns the refreshed runtime settings.
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  FederationUpdateDTO  true  "Instance name and sqid"
// @Success      200  {object}  SettingsDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      409  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation [patch]
func (h *Handler) handleFederationUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Federation == nil || h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	var body struct {
		Name string `json:"name"`
		Sqid string `json:"sqid"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorParams(w, http.StatusBadRequest, "invalid_body", "invalid JSON: "+err.Error(), map[string]any{"detail": err.Error()})
		return
	}
	if err := h.Federation.UpdateInstance(r.Context(), body.Name, body.Sqid); err != nil {
		writeError(w, http.StatusBadGateway, "update_failed", err.Error())
		return
	}
	writeResource(w, http.StatusOK, settingsBody(redactSettings(h.Settings.Get()), h.Settings.PendingRestart()))
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
