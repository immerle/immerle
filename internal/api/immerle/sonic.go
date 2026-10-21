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
// @Success      200  {object}  songListView
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
	writeResource(w, http.StatusOK, songList(trackEntriesToSongViews(tracks)))
}

// maxAutoDJTracks bounds an Auto-DJ request (the ordering is quadratic).
const maxAutoDJTracks = 500

// AutoDJRequestDTO is the body of POST /songs/autodj.
type AutoDJRequestDTO struct {
	// IDs are the track ids to order (a playlist, or the queue from the
	// current track on). Duplicates are kept.
	IDs []string `json:"ids" validate:"optional"`
	// KeepFirst pins ids[0], e.g. the track playing now.
	KeepFirst bool `json:"keepFirst" example:"true" validate:"optional"`
}

// AutoDJResponseDTO is the same ids, reordered.
type AutoDJResponseDTO struct {
	IDs []string `json:"ids" validate:"optional"`
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

// SonicDuplicatesDTO lists groups of tracks that sound identical.
type SonicDuplicatesDTO struct {
	Groups [][]songView `json:"groups" validate:"optional"`
}

// handleSonicDuplicates lists probable duplicates.
//
// @Summary      Sonic duplicates
// @Description  Admin only. Groups of analyzed tracks that sound identical and last about as long (within 3 s): the same recording under other tags, or in another format. Nothing is deleted; the admin picks which copy to keep. 404 when the sonic duplicates setting is off.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  SonicDuplicatesDTO
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /admin/sonic/duplicates [get]
func (h *Handler) handleSonicDuplicates(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if !h.Sonic.DuplicatesEnabled() {
		writeError(w, http.StatusNotFound, "not_found", "sonic duplicates are disabled")
		return
	}
	groups, err := h.Sonic.Duplicates(r.Context())
	if err != nil {
		writeInternal(w, err)
		return
	}
	var ids []string
	for _, g := range groups {
		ids = append(ids, g...)
	}
	byID, err := h.Catalog.TracksByIDs(r.Context(), ids)
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := SonicDuplicatesDTO{Groups: [][]songView{}}
	for _, g := range groups {
		var views []songView
		for _, id := range g {
			if t, ok := byID[id]; ok {
				views = append(views, toSongView(t))
			}
		}
		if len(views) > 1 { // a copy deleted since the index was built
			out.Groups = append(out.Groups, views)
		}
	}
	writeResource(w, http.StatusOK, out)
}

// SongSonicDTO is a track's audio analysis.
type SongSonicDTO struct {
	BPM          float64 `json:"bpm" example:"127.6"`
	Key          string  `json:"key" example:"A"`
	Mode         string  `json:"mode" example:"minor"`
	Camelot      string  `json:"camelot" example:"8A"`
	Energy       float64 `json:"energy" example:"0.82"`
	Danceability float64 `json:"danceability" example:"0.64"`
}

// handleSongSonic returns a track's audio analysis.
//
// @Summary      Track audio analysis
// @Description  The tempo, key and energy the local audio analysis measured for a track (energy and danceability from 0 to 1). 404 when the analysis is switched off or the track isn't analyzed yet.
// @Tags         browse
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Track id"
// @Success      200  {object}  SongSonicDTO
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /songs/{id}/sonic [get]
func (h *Handler) handleSongSonic(w http.ResponseWriter, r *http.Request) {
	if !h.Sonic.Enabled() {
		writeError(w, http.StatusNotFound, "not_found", "audio analysis is disabled")
		return
	}
	f, ok := h.Sonic.Features(r.Context(), pathParam(r, "id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "track not analyzed")
		return
	}
	writeResource(w, http.StatusOK, SongSonicDTO{
		BPM: f.BPM, Key: f.Key, Mode: f.Mode, Camelot: f.Camelot, Energy: f.Energy, Danceability: f.Danceability,
	})
}
