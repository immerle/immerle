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

// maxAutoDJTracks bounds an Auto-DJ request (the ordering is quadratic).
const maxAutoDJTracks = 500

// AutoDJRequestDTO is the body of POST /songs/autodj.
type AutoDJRequestDTO struct {
	// IDs are the track ids to order (a playlist, or the queue from the
	// current track on). Duplicates are kept.
	IDs []string `json:"ids"`
	// KeepFirst pins ids[0], e.g. the track playing now.
	KeepFirst bool `json:"keepFirst" example:"true"`
}

// AutoDJResponseDTO is the same ids, reordered.
type AutoDJResponseDTO struct {
	IDs []string `json:"ids"`
}

// handleAutoDJ orders tracks for smooth transitions.
//
// @Summary      Auto-DJ order
// @Description  Reorders tracks for smooth transitions from the local audio analysis: close tempos (a half or double tempo counts as close), harmonically compatible keys on the Camelot wheel, small energy jumps. Starts from ids[0] when keepFirst, else from the calmest track. Tracks not analyzed yet follow, in their original order. 404 when Auto-DJ is switched off.
// @Tags         browse
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  AutoDJRequestDTO  true  "Tracks to order"
// @Success      200  {object}  AutoDJResponseDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /songs/autodj [post]
func (h *Handler) handleAutoDJ(w http.ResponseWriter, r *http.Request) {
	if !h.Sonic.AutoDJEnabled() {
		writeError(w, http.StatusNotFound, "not_found", "Auto-DJ is disabled")
		return
	}
	var req AutoDJRequestDTO
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > maxAutoDJTracks {
		writeValidation(w, []fieldError{{Field: "ids", Message: "between 1 and 500 track ids are required"}})
		return
	}
	ids, err := h.Sonic.AutoDJ(r.Context(), req.IDs, req.KeepFirst)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, AutoDJResponseDTO{IDs: ids})
}
