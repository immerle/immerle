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
	if pid, ok := strings.CutPrefix(id, playlistCoverPrefix); ok {
		if !h.servePlaylistCover(w, r, pid) {
			http.Error(w, "Cover art not found", http.StatusNotFound)
		}
		return
	}
	err := h.media.ServeCover(w, r, id, intParam(r, "size", 0))
	if err == nil {
		return
	}
	if errors.Is(err, stream.ErrNoCover) {
		// Some clients (e.g. Arpeggi) ignore the playlist coverArt field and
		// ask for the playlist id itself.
		if !h.servePlaylistCover(w, r, id) {
			http.Error(w, "Cover art not found", http.StatusNotFound)
		}
		return
	}
	if h.Logger != nil {
		h.Logger.Error("getCoverArt failed", "id", id, "error", err)
	}
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

// servePlaylistCover serves a playlist's custom cover, else its track mosaic.
// False when id is no playlist or it has no cover at all.
func (h *Handler) servePlaylistCover(w http.ResponseWriter, r *http.Request, id string) bool {
	if h.Playlists == nil {
		return false
	}
	p, err := h.Playlists.Get(r.Context(), id)
	if err != nil {
		return false
	}
	if p.CoverArt != "" {
		return h.media.ServeCover(w, r, p.CoverArt, intParam(r, "size", 0)) == nil
	}
	return h.serveMosaic(w, r, p.CoverArts)
}

// serveMosaic renders a playlist's track covers as the same 2x2 mosaic the UI
// shows. ponytail: rendered per request, cache it if getCoverArt gets hot.
func (h *Handler) serveMosaic(w http.ResponseWriter, r *http.Request, coverIDs []string) bool {
	size := intParam(r, "size", 0)
	tiles := make([][]byte, 0, len(coverIDs))
	for _, id := range coverIDs {
		if data, _, err := h.Cover.Get(r.Context(), id, size, ""); err == nil {
			tiles = append(tiles, data)
		}
	}
	data, contentType, err := covergen.RenderMosaic(tiles)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", contentType)
	// Shorter than other covers: it changes as soon as the first tracks do.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(data)
	return true
}
