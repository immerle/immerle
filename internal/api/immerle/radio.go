package immerle

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/radio"
)

// radioCoverClient fetches station logos with a bounded timeout.
var radioCoverClient = &http.Client{Timeout: 12 * time.Second}

// maxRadioCoverBytes caps a fetched station logo (5 MiB).
const maxRadioCoverBytes = 5 << 20

// radioEnabled reports whether internet radio is on (default on when settings
// are unavailable, e.g. in tests).
func (h *Handler) radioEnabled() bool {
	return h.Settings == nil || h.Settings.RadioEnabled()
}

// radioPathID returns the {id} path param, percent-decoded so station ids that
// contain ':' (e.g. "builtin:nrj") match whether or not the client encoded them
// (chi does not decode path params).
func radioPathID(r *http.Request) string {
	id := pathParam(r, "id")
	if dec, err := url.PathUnescape(id); err == nil {
		return dec
	}
	return id
}

type stationView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	StreamURL   string `json:"streamUrl"`
	HomepageURL string `json:"homepageUrl"`
	Country     string `json:"country"`
	Builtin     bool   `json:"builtin"`
	// Deletable is false for built-in stations (they can be edited, not removed).
	Deletable bool `json:"deletable"`
	// HasCover tells clients whether to load the station cover endpoint.
	HasCover bool `json:"hasCover"`
	// CoverURL is the external logo source URL (for prefilling the admin edit
	// form). Empty for built-ins whose logo is an embedded asset.
	CoverURL string `json:"coverUrl"`
	// Liked is true when the caller has favorited this station.
	Liked bool `json:"liked"`
}

func toStationView(s models.RadioStation) stationView {
	coverURL := ""
	if strings.HasPrefix(s.CoverArt, "http") {
		coverURL = s.CoverArt
	}
	return stationView{
		ID: s.ID, Name: s.Name, StreamURL: s.StreamURL, HomepageURL: s.HomepageURL,
		Country: s.Country, Builtin: s.Builtin, Deletable: !s.Builtin,
		HasCover: s.CoverArt != "", CoverURL: coverURL, Liked: s.Liked,
	}
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
	stations, err := h.Radio.ListForUser(r.Context(), userFrom(r.Context()).ID)
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

// handleRadioLike favorites a station for the caller.
//
// @Summary      Like a radio station
// @Tags         radio
// @Security     BearerAuth
// @Param        id  path  string  true  "Station id"
// @Success      204
// @Failure      404  {object}  errorResponse
// @Router       /radio/stations/{id}/like [put]
func (h *Handler) handleRadioLike(w http.ResponseWriter, r *http.Request) { h.setRadioLike(w, r, true) }

// handleRadioUnlike removes a station from the caller's favorites.
//
// @Summary      Unlike a radio station
// @Tags         radio
// @Security     BearerAuth
// @Param        id  path  string  true  "Station id"
// @Success      204
// @Router       /radio/stations/{id}/like [delete]
func (h *Handler) handleRadioUnlike(w http.ResponseWriter, r *http.Request) {
	h.setRadioLike(w, r, false)
}

func (h *Handler) setRadioLike(w http.ResponseWriter, r *http.Request, liked bool) {
	if !h.radioEnabled() || h.Radio == nil {
		writeError(w, http.StatusNotFound, "disabled", "radio is disabled")
		return
	}
	id := radioPathID(r)
	if _, err := h.Radio.Get(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "station not found")
		return
	}
	if err := h.Radio.SetLiked(r.Context(), userFrom(r.Context()).ID, id, liked); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// radioRequest is the admin create/update body.
type radioRequest struct {
	Name        string `json:"name"`
	StreamURL   string `json:"streamUrl"`
	HomepageURL string `json:"homepageUrl"`
	// CoverURL is the station logo source URL (fetched + cached server-side).
	CoverURL string `json:"coverUrl"`
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
		HomepageURL: strings.TrimSpace(req.HomepageURL), CoverArt: strings.TrimSpace(req.CoverURL), CreatedAt: now, UpdatedAt: now,
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
	st, err := h.Radio.Get(r.Context(), radioPathID(r))
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
	newCover := strings.TrimSpace(req.CoverURL)
	if newCover != st.CoverArt {
		// The logo changed: drop the stale cached image so the next request
		// re-fetches from the new source.
		_ = os.Remove(radioCoverPath(h.CoversDir, st.ID))
		st.CoverArt = newCover
	}
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
	id := radioPathID(r)
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
	_ = os.Remove(radioCoverPath(h.CoversDir, id)) // drop the cached logo too
	writeResource(w, http.StatusNoContent, nil)
}

// --- station logo (cached + served locally) ---

// radioCoverPath is the on-disk cache path for a station's logo. The id may
// contain ':' (e.g. "builtin:nrj"), so it is sanitized for the filesystem.
func radioCoverPath(coversDir, id string) string {
	safe := strings.NewReplacer("/", "_", ":", "_", "\\", "_").Replace(id)
	return filepath.Join(coversDir, "radio", safe)
}

// handleRadioCover serves a station's logo. On the first request it fetches the
// station's source URL and caches the bytes locally; later requests serve the
// cached file. Public (logos aren't sensitive) so clients load it as a plain
// image without auth headers, and the server fetches http-only logos for clients
// on https (no mixed-content).
//
// @Summary      Station logo
// @Tags         radio
// @Produce      image/png
// @Param        id  path  string  true  "Station id"
// @Success      200
// @Failure      404  {object}  errorResponse
// @Router       /radio/stations/{id}/cover [get]
func (h *Handler) handleRadioCover(w http.ResponseWriter, r *http.Request) {
	if h.Radio == nil {
		http.NotFound(w, r)
		return
	}
	id := radioPathID(r)
	path := radioCoverPath(h.CoversDir, id)
	if _, err := os.Stat(path); err == nil {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, path)
		return
	}
	st, err := h.Radio.Get(r.Context(), id)
	if err != nil || st.CoverArt == "" {
		http.NotFound(w, r)
		return
	}
	// Built-in logos are embedded in the binary — serve them straight from there.
	if data, ctype, ok := radio.CoverFile(st.CoverArt); ok {
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "public, max-age=604800")
		_, _ = w.Write(data)
		return
	}
	data, ctype, err := fetchRadioCover(r.Context(), st.CoverArt)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

// fetchRadioCover downloads a logo, enforcing an image content-type and a size cap.
func fetchRadioCover(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "immerle")
	resp, err := radioCoverClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("cover fetch %s: %s", url, resp.Status)
	}
	ctype := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ctype, "image/") {
		return nil, "", fmt.Errorf("cover is not an image: %s", ctype)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRadioCoverBytes))
	if err != nil {
		return nil, "", err
	}
	return data, ctype, nil
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
