package subsonic

import (
	"errors"
	"net/http"
	"strings"

	"github.com/immerle/immerle/internal/stream"
)

func (h *Handler) handleGetCoverArt(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if pid, ok := strings.CutPrefix(id, playlistCoverPrefix); ok && h.Playlists != nil {
		if p, err := h.Playlists.Get(r.Context(), pid); err == nil && p.CoverArt != "" {
			id = p.CoverArt
		}
	}
	if err := h.media.ServeCover(w, r, id, intParam(r, "size", 0)); err != nil {
		if errors.Is(err, stream.ErrNoCover) {
			http.Error(w, "Cover art not found", http.StatusNotFound)
			return
		}
		if h.Logger != nil {
			h.Logger.Error("getCoverArt failed", "id", id, "error", err)
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
