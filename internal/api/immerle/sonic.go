package immerle

import (
	"net/http"
	"strings"
)

// SonicStatusDTO is the local audio-analysis state and progress.
type SonicStatusDTO struct {
	// Available is false when ffmpeg is missing: analysis can't run at all.
	Available bool `json:"available" example:"true"`
	Enabled   bool `json:"enabled" example:"true"`
	// Total counts local tracks; Analyzed and Failed those processed with the
	// current algorithm.
	Total    int `json:"total" example:"12000"`
	Analyzed int `json:"analyzed" example:"4210"`
	Failed   int `json:"failed" example:"3"`
}

// handleSonicStatus reports the audio-analysis progress.
//
// @Summary      Audio analysis status
// @Description  Admin only. Whether local audio analysis can run (ffmpeg found) and is switched on (sonic.enabled setting), plus its progress over the local library.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  SonicStatusDTO
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/sonic [get]
func (h *Handler) handleSonicStatus(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Sonic == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "audio analysis not available")
		return
	}
	st, err := h.Sonic.Stats(r.Context())
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, SonicStatusDTO{
		Available: h.Sonic.Available(),
		Enabled:   h.Sonic.Enabled(),
		Total:     st.Total,
		Analyzed:  st.Analyzed,
		Failed:    st.Failed,
	})
}

// handleSonicRadio returns tracks that sound like the seeds.
//
// @Summary      Sonic radio
// @Description  Tracks that sound like the seeds, from the local audio analysis: the closest analyzed tracks, at most two per artist, with a little randomness so two radios differ. A seed is a track, album or artist id; several seeds blend into one radio (e.g. the last tracks played, to extend a queue). 404 when both the sonic radio and autoplay are switched off; an empty list when none of the seeds is analyzed yet.
// @Tags         browse
// @Security     BearerAuth
// @Produce      json
// @Param        seed     query  []string  true   "Track, album or artist id (repeatable)"  collectionFormat(multi)
// @Param        exclude  query  string    false  "Comma-separated track ids to leave out (e.g. the current queue)"
// @Param        count    query  int       false  "Number of tracks (default 50, max 200)"
// @Success      200  {object}  map[string][]songView
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /songs/radio [get]
func (h *Handler) handleSonicRadio(w http.ResponseWriter, r *http.Request) {
	if !h.Sonic.RadioEnabled() && !h.Sonic.AutoplayEnabled() {
		writeError(w, http.StatusNotFound, "not_found", "sonic radio is disabled")
		return
	}
	seeds := r.URL.Query()["seed"]
	if len(seeds) == 0 {
		writeValidation(w, []fieldError{{Field: "seed", Message: "at least one seed is required"}})
		return
	}
	var exclude []string
	if raw := r.URL.Query().Get("exclude"); raw != "" {
		exclude = strings.Split(raw, ",")
	}
	count := min(max(intQuery(r, "count", 50), 1), 200)
	tracks, _ := h.library.SonicRadio(r.Context(), userFrom(r.Context()).ID, seeds, exclude, count)
	writeResource(w, http.StatusOK, map[string]any{"songs": trackEntriesToSongViews(tracks)})
}
