package subsonic

import (
	"errors"
	"net/http"

	"github.com/immerle/immerle/internal/stream"
)

func (h *Handler) handleGetCoverArt(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	size := intParam(r, "size", 0)

	data, contentType, err := h.Cover.Get(r.Context(), id, size)
	if err != nil {
		if errors.Is(err, stream.ErrNoCover) {
			http.Error(w, "Cover art not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
