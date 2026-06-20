package immerle

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
)

// smartEnabled reports whether rule-based playlists are on (default on when
// settings are unavailable, e.g. in tests).
func (h *Handler) smartEnabled() bool {
	return h.Settings == nil || h.Settings.SmartPlaylistsEnabled()
}

// smartUnavailable answers 404 when the feature is off or unwired. Returns true
// when the request was handled (caller should stop).
func (h *Handler) smartUnavailable(w http.ResponseWriter) bool {
	if h.smartEnabled() && h.SmartPlaylists != nil {
		return false
	}
	writeError(w, http.StatusNotFound, "disabled", "smart playlists are disabled")
	return true
}

type smartPlaylistView struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Rules     models.SmartRules `json:"rules"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

func toSmartView(sp models.SmartPlaylist) smartPlaylistView {
	return smartPlaylistView{ID: sp.ID, Name: sp.Name, Rules: sp.Rules, CreatedAt: sp.CreatedAt, UpdatedAt: sp.UpdatedAt}
}

// smartRequest is the create/update/preview body.
type smartRequest struct {
	Name  string            `json:"name"`
	Rules models.SmartRules `json:"rules"`
}

// handleSmartPlaylists lists the caller's smart playlists.
//
// @Summary      List smart playlists
// @Tags         smartPlaylists
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /smart-playlists [get]
func (h *Handler) handleSmartPlaylists(w http.ResponseWriter, r *http.Request) {
	if h.smartUnavailable(w) {
		return
	}
	list, err := h.SmartPlaylists.ListByOwner(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	views := make([]smartPlaylistView, 0, len(list))
	for _, sp := range list {
		views = append(views, toSmartView(sp))
	}
	writeResource(w, http.StatusOK, map[string]any{"playlists": views})
}

// handleSmartPlaylistCreate creates a smart playlist.
//
// @Summary      Create a smart playlist
// @Tags         smartPlaylists
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  SmartPlaylistRequestDTO  true  "Name + rules"
// @Success      201  {object}  SmartPlaylistDTO
// @Failure      400  {object}  errorResponse
// @Router       /smart-playlists [post]
func (h *Handler) handleSmartPlaylistCreate(w http.ResponseWriter, r *http.Request) {
	if h.smartUnavailable(w) {
		return
	}
	var req smartRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "validation", "name is required")
		return
	}
	now := time.Now()
	sp := models.SmartPlaylist{
		ID: uuid.NewString(), OwnerID: userFrom(r.Context()).ID,
		Name: strings.TrimSpace(req.Name), Rules: req.Rules, CreatedAt: now, UpdatedAt: now,
	}
	if err := h.SmartPlaylists.Create(r.Context(), sp); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusCreated, toSmartView(sp))
}

// handleSmartPlaylistUpdate replaces a smart playlist's name and rules.
//
// @Summary      Update a smart playlist
// @Tags         smartPlaylists
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  string        true  "Smart playlist id"
// @Param        body  body  SmartPlaylistRequestDTO  true  "Name + rules"
// @Success      200  {object}  SmartPlaylistDTO
// @Failure      404  {object}  errorResponse
// @Router       /smart-playlists/{id} [put]
func (h *Handler) handleSmartPlaylistUpdate(w http.ResponseWriter, r *http.Request) {
	if h.smartUnavailable(w) {
		return
	}
	owner := userFrom(r.Context()).ID
	existing, err := h.SmartPlaylists.Get(r.Context(), pathParam(r, "id"), owner)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "smart playlist not found")
		return
	}
	var req smartRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		existing.Name = strings.TrimSpace(req.Name)
	}
	existing.Rules = req.Rules
	existing.UpdatedAt = time.Now()
	if err := h.SmartPlaylists.Update(r.Context(), existing); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, toSmartView(existing))
}

// handleSmartPlaylistDelete removes a smart playlist.
//
// @Summary      Delete a smart playlist
// @Tags         smartPlaylists
// @Security     BearerAuth
// @Param        id  path  string  true  "Smart playlist id"
// @Success      204
// @Router       /smart-playlists/{id} [delete]
func (h *Handler) handleSmartPlaylistDelete(w http.ResponseWriter, r *http.Request) {
	if h.smartUnavailable(w) {
		return
	}
	if err := h.SmartPlaylists.Delete(r.Context(), pathParam(r, "id"), userFrom(r.Context()).ID); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// handleSmartPlaylistTracks resolves a saved smart playlist into its current
// tracks (Subsonic-compatible song shape, ready to enqueue).
//
// @Summary      Resolve a smart playlist's tracks
// @Tags         smartPlaylists
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Smart playlist id"
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  errorResponse
// @Router       /smart-playlists/{id}/tracks [get]
func (h *Handler) handleSmartPlaylistTracks(w http.ResponseWriter, r *http.Request) {
	if h.smartUnavailable(w) {
		return
	}
	user := userFrom(r.Context())
	sp, err := h.SmartPlaylists.Get(r.Context(), pathParam(r, "id"), user.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "smart playlist not found")
		return
	}
	h.writeSmartTracks(w, r, sp.Rules, user.ID)
}

// handleSmartPlaylistPreview evaluates ad-hoc rules without saving (for the
// editor's live preview).
//
// @Summary      Preview smart-playlist rules
// @Tags         smartPlaylists
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  SmartPlaylistRequestDTO  true  "Rules to preview"
// @Success      200  {object}  map[string]interface{}
// @Router       /smart-playlists/preview [post]
func (h *Handler) handleSmartPlaylistPreview(w http.ResponseWriter, r *http.Request) {
	if h.smartUnavailable(w) {
		return
	}
	var req smartRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	h.writeSmartTracks(w, r, req.Rules, userFrom(r.Context()).ID)
}

func (h *Handler) writeSmartTracks(w http.ResponseWriter, r *http.Request, rules models.SmartRules, userID string) {
	tracks, err := h.SmartPlaylists.Evaluate(r.Context(), rules, userID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	songs := make([]songView, 0, len(tracks))
	for _, t := range tracks {
		songs = append(songs, toSongView(t))
	}
	writeResource(w, http.StatusOK, map[string]any{"songs": songs})
}

// --- admin toggle ---

func (h *Handler) smartStatus() map[string]any { return map[string]any{"enabled": h.smartEnabled()} }

// handleSmartPlaylistsAdmin reports whether the feature is enabled.
//
// @Summary      Get the smart-playlists feature state
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string]bool
// @Router       /admin/smart-playlists [get]
func (h *Handler) handleSmartPlaylistsAdmin(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	writeResource(w, http.StatusOK, h.smartStatus())
}

// handleSmartPlaylistsToggle turns rule-based playlists on or off (hot).
//
// @Summary      Toggle the smart-playlists feature
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  smartToggleRequest  true  "Enable or disable"
// @Success      200  {object}  map[string]bool
// @Failure      400  {object}  errorResponse
// @Router       /admin/smart-playlists [put]
func (h *Handler) handleSmartPlaylistsToggle(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings not available")
		return
	}
	var req smartToggleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "validation", "enabled is required")
		return
	}
	next := h.Settings.Get()
	next.SmartPlaylists.Enabled = *req.Enabled
	if _, _, err := h.Settings.Update(next); err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("smart playlists toggled", "enabled", *req.Enabled, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, h.smartStatus())
}

// smartToggleRequest is the admin on/off body.
type smartToggleRequest struct {
	Enabled *bool `json:"enabled"`
}
