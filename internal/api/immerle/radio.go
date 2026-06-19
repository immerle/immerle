package immerle

import (
	"net/http"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// radioEnabled reports whether internet radio is on (default on when settings
// are unavailable, e.g. in tests).
func (h *Handler) radioEnabled() bool {
	return h.Settings == nil || h.Settings.RadioEnabled()
}

type stationView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	StreamURL   string `json:"streamUrl"`
	HomepageURL string `json:"homepageUrl"`
	Builtin     bool   `json:"builtin"`
	// Deletable is false for built-in stations (they can be edited, not removed).
	Deletable bool `json:"deletable"`
}

func toStationView(s models.RadioStation) stationView {
	return stationView{ID: s.ID, Name: s.Name, StreamURL: s.StreamURL, HomepageURL: s.HomepageURL, Builtin: s.Builtin, Deletable: !s.Builtin}
}

// handleRadioList lists the radio stations (any authenticated user).
//
// @Summary      List internet radio stations
// @Tags         radio
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  errorResponse
// @Router       /radio [get]
func (h *Handler) handleRadioList(w http.ResponseWriter, r *http.Request) {
	if !h.radioEnabled() || h.Radio == nil {
		writeError(w, http.StatusNotFound, "disabled", "radio is disabled")
		return
	}
	stations, err := h.Radio.List(r.Context())
	if err != nil {
		writeInternal(w, err)
		return
	}
	views := make([]stationView, 0, len(stations))
	for _, s := range stations {
		views = append(views, toStationView(s))
	}
	writeResource(w, http.StatusOK, map[string]any{"stations": views})
}

// radioRequest is the admin create/update body.
type radioRequest struct {
	Name        string `json:"name"`
	StreamURL   string `json:"streamUrl"`
	HomepageURL string `json:"homepageUrl"`
}

func (req radioRequest) valid() bool {
	return strings.TrimSpace(req.Name) != "" && strings.HasPrefix(strings.TrimSpace(req.StreamURL), "http")
}

// handleRadioCreate adds a custom station (admin only).
//
// @Summary      Create a radio station
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  radioRequest  true  "Station"
// @Success      201  {object}  stationView
// @Failure      400  {object}  errorResponse
// @Router       /admin/radio/stations [post]
func (h *Handler) handleRadioCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || h.Radio == nil {
		if h.Radio == nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "radio not available")
		}
		return
	}
	var req radioRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !req.valid() {
		writeError(w, http.StatusBadRequest, "validation", "name and a http(s) streamUrl are required")
		return
	}
	now := time.Now()
	st := models.RadioStation{
		ID: persistence.NewStationID(), Name: strings.TrimSpace(req.Name), StreamURL: strings.TrimSpace(req.StreamURL),
		HomepageURL: strings.TrimSpace(req.HomepageURL), CreatedAt: now, UpdatedAt: now,
	}
	if err := h.Radio.Create(r.Context(), st); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusCreated, toStationView(st))
}

// handleRadioUpdate edits a station (admin only; built-ins are editable).
//
// @Summary      Update a radio station
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  string        true  "Station id"
// @Param        body  body  radioRequest  true  "Station"
// @Success      200  {object}  stationView
// @Failure      404  {object}  errorResponse
// @Router       /admin/radio/stations/{id} [put]
func (h *Handler) handleRadioUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || h.Radio == nil {
		if h.Radio == nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "radio not available")
		}
		return
	}
	st, err := h.Radio.Get(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "station not found")
		return
	}
	var req radioRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		st.Name = strings.TrimSpace(req.Name)
	}
	if strings.HasPrefix(strings.TrimSpace(req.StreamURL), "http") {
		st.StreamURL = strings.TrimSpace(req.StreamURL)
	}
	st.HomepageURL = strings.TrimSpace(req.HomepageURL)
	st.UpdatedAt = time.Now()
	if err := h.Radio.Update(r.Context(), st); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, toStationView(st))
}

// handleRadioDelete removes a custom station (admin only; built-ins can't be
// deleted).
//
// @Summary      Delete a radio station
// @Tags         admin
// @Security     BearerAuth
// @Param        id  path  string  true  "Station id"
// @Success      204
// @Failure      400  {object}  errorResponse
// @Router       /admin/radio/stations/{id} [delete]
func (h *Handler) handleRadioDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || h.Radio == nil {
		if h.Radio == nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "radio not available")
		}
		return
	}
	id := pathParam(r, "id")
	st, err := h.Radio.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "station not found")
		return
	}
	if st.Builtin {
		writeError(w, http.StatusBadRequest, "validation", "built-in stations cannot be deleted")
		return
	}
	if err := h.Radio.Delete(r.Context(), id); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// --- feature toggle ---

func (h *Handler) radioStatus() map[string]any { return map[string]any{"enabled": h.radioEnabled()} }

// handleRadioAdmin reports whether the radio feature is enabled.
//
// @Summary      Get the radio feature state
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string]bool
// @Router       /admin/radio [get]
func (h *Handler) handleRadioAdmin(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	writeResource(w, http.StatusOK, h.radioStatus())
}

// handleRadioToggle turns internet radio on or off (hot).
//
// @Summary      Toggle the radio feature
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  radioToggleRequest  true  "Enable or disable"
// @Success      200  {object}  map[string]bool
// @Failure      400  {object}  errorResponse
// @Router       /admin/radio [put]
func (h *Handler) handleRadioToggle(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings not available")
		return
	}
	var req radioToggleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "validation", "enabled is required")
		return
	}
	next := h.Settings.Get()
	next.Radio.Enabled = *req.Enabled
	if _, _, err := h.Settings.Update(next); err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("radio toggled", "enabled", *req.Enabled, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, h.radioStatus())
}

// radioToggleRequest is the admin on/off body.
type radioToggleRequest struct {
	Enabled *bool `json:"enabled"`
}
