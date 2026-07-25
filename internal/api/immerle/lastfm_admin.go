package immerle

import (
	"net/http"

	"github.com/immerle/immerle/internal/models"
)

// lastFmStatus is the admin view of Last.fm scrobbling config. The API
// key/secret themselves are write-only (see redactSettings) — this only
// reports whether they're set.
func (h *Handler) lastFmStatus() LastFmAdminDTO {
	cfg := models.LastFmRuntime{}
	if h.Settings != nil {
		cfg = h.Settings.Get().LastFm
	}
	return LastFmAdminDTO{Enabled: cfg.Enabled, Configured: cfg.APIKey != "" && cfg.APISecret != ""}
}

// handleLastFmAdmin reports the Last.fm scrobbling feature state.
//
// @Summary  Get the Last.fm scrobbling feature state
// @Tags     admin
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  LastFmAdminDTO
// @Router   /admin/lastfm [get]
func (h *Handler) handleLastFmAdmin(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	writeResource(w, http.StatusOK, h.lastFmStatus())
}

// lastFmUpdateRequest is a partial update of Last.fm settings; pointer fields
// distinguish "omitted" (keep current) from "" (clear).
type lastFmUpdateRequest struct {
	Enabled   *bool   `json:"enabled"`
	APIKey    *string `json:"apiKey"`
	APISecret *string `json:"apiSecret"`
}

// handleLastFmAdminUpdate changes Last.fm scrobbling settings: enable/disable
// and set the app-level API key/shared secret (hot-reloaded, no restart
// needed).
//
// @Summary  Update Last.fm scrobbling settings
// @Description  Admin only. Partial update — only fields present are changed. The API key/secret are write-only: the response never echoes them back.
// @Tags     admin
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body  body  lastFmUpdateRequest  true  "Fields to change"
// @Success  200  {object}  LastFmAdminDTO
// @Failure  400  {object}  errorResponse
// @Router   /admin/lastfm [put]
func (h *Handler) handleLastFmAdminUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings not available")
		return
	}
	var req lastFmUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	next := h.Settings.Get()
	if req.Enabled != nil {
		next.LastFm.Enabled = *req.Enabled
	}
	if req.APIKey != nil {
		next.LastFm.APIKey = *req.APIKey
	}
	if req.APISecret != nil {
		next.LastFm.APISecret = *req.APISecret
	}
	if _, _, err := h.Settings.Update(next); err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("lastfm settings updated", "enabled", next.LastFm.Enabled, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, h.lastFmStatus())
}
