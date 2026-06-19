package subsonic

import (
	"net/http"
)

func (h *Handler) handleStar(w http.ResponseWriter, r *http.Request) {
	h.setStar(w, r, true)
}

func (h *Handler) handleUnstar(w http.ResponseWriter, r *http.Request) {
	h.setStar(w, r, false)
}

// setStar applies (un)starring to the id/albumId/artistId parameters.
func (h *Handler) setStar(w http.ResponseWriter, r *http.Request, star bool) {
	h.playback.SetStarred(r.Context(), userFrom(r.Context()), r.Form["id"], r.Form["albumId"], r.Form["artistId"], star)
	writeOK(w, r)
}

func (h *Handler) handleSetRating(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if id == "" {
		writeError(w, r, ErrMissingParameter, "Required parameter id is missing")
		return
	}
	if err := h.playback.SetRating(r.Context(), userFrom(r.Context()).ID, id, intParam(r, "rating", 0)); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}
