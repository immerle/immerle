package immerle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/api/httputil"
	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
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
	// Country is the station group code (e.g. "fr", "gb", "int").
	Country string `json:"country"`
	Builtin bool   `json:"builtin"`
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

// stationListView is the GET /radio body (stations is never null).
type stationListView struct {
	Stations []stationView `json:"stations" validate:"required"`
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
// @Success      200  {object}  stationListView
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
	writeResource(w, http.StatusOK, stationListView{Stations: views})
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
		writeLookupError(w, err, "station not found")
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
	Name        string `json:"name" validate:"optional"`
	StreamURL   string `json:"streamUrl" validate:"optional"`
	HomepageURL string `json:"homepageUrl" validate:"optional"`
	// CoverURL is the station logo source URL (fetched + cached server-side).
	CoverURL string `json:"coverUrl" validate:"optional"`
}

// writeRadioError maps a core.RadioService error to its REST response.
func writeRadioError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrInvalidStation), errors.Is(err, core.ErrBuiltinStation):
		writeError(w, http.StatusBadRequest, "validation", err.Error())
	default:
		writeLookupError(w, err, "station not found")
	}
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
	if h.Radio == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "radio not available")
		return
	}
	var req radioRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	st, err := h.Services.Radio.Create(r.Context(), models.RadioStation{
		Name: req.Name, StreamURL: req.StreamURL, HomepageURL: req.HomepageURL, CoverArt: req.CoverURL,
	})
	if err != nil {
		writeRadioError(w, err)
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
	if h.Radio == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "radio not available")
		return
	}
	var req radioRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	before, st, err := h.Services.Radio.Update(r.Context(), radioPathID(r), func(st *models.RadioStation) {
		if strings.TrimSpace(req.Name) != "" {
			st.Name = req.Name
		}
		if strings.HasPrefix(strings.TrimSpace(req.StreamURL), "http") {
			st.StreamURL = req.StreamURL
		}
		st.HomepageURL = req.HomepageURL
		st.CoverArt = req.CoverURL
	})
	if err != nil {
		writeRadioError(w, err)
		return
	}
	if st.CoverArt != before.CoverArt {
		// The logo changed: drop the stale cached image so the next request
		// re-fetches from the new source.
		_ = os.Remove(radioCoverPath(h.CoversDir, st.ID))
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
	if h.Radio == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "radio not available")
		return
	}
	id := radioPathID(r)
	if err := h.Services.Radio.Delete(r.Context(), id); err != nil {
		writeRadioError(w, err)
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

// handleRadioCover serves a station's logo, fetching and caching it locally on
// first request. Public (logos aren't sensitive) so clients load it as a plain
// <img>; fetching server-side also avoids mixed-content for http-only logos on https.
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
	if err := httputil.ValidateFetchURL(ctx, url); err != nil {
		return nil, "", err
	}
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

func (h *Handler) radioStatus() featureStatusView {
	return featureStatusView{Enabled: h.radioEnabled()}
}

// handleRadioAdmin reports whether the radio feature is enabled.
//
// @Summary      Get the radio feature state
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  featureStatusView
// @Router       /admin/radio [get]
func (h *Handler) handleRadioAdmin(w http.ResponseWriter, r *http.Request) {
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
// @Success      200  {object}  featureStatusView
// @Failure      400  {object}  errorResponse
// @Router       /admin/radio [put]
func (h *Handler) handleRadioToggle(w http.ResponseWriter, r *http.Request) {
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
	Enabled *bool `json:"enabled,omitempty" validate:"optional"`
}
