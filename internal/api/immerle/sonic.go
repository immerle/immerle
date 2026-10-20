package immerle

import "net/http"

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
