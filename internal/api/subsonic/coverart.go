package subsonic

import (
	"errors"
	"net/http"
	"strings"

	"github.com/immerle/immerle/internal/covergen"
	"github.com/immerle/immerle/internal/stream"
)

func (h *Handler) handleGetCoverArt(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if pid, ok := strings.CutPrefix(id, playlistCoverPrefix); ok && h.Playlists != nil {
		if p, err := h.Playlists.Get(r.Context(), pid); err == nil {
			if p.CoverArt == "" {
				h.serveMosaic(w, r, p.CoverArts)
				return
			}
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

// serveMosaic renders a playlist's track covers as the same 2x2 mosaic the UI
// shows. ponytail: rendered per request, cache it if getCoverArt gets hot.
func (h *Handler) serveMosaic(w http.ResponseWriter, r *http.Request, coverIDs []string) {
	size := intParam(r, "size", 0)
	tiles := make([][]byte, 0, len(coverIDs))
	for _, id := range coverIDs {
		if data, _, err := h.Cover.Get(r.Context(), id, size, ""); err == nil {
			tiles = append(tiles, data)
		}
	}
	data, contentType, err := covergen.RenderMosaic(tiles)
	if err != nil {
		http.Error(w, "Cover art not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	// Shorter than other covers: it changes as soon as the first tracks do.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(data)
}
