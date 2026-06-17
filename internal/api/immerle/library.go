package immerle

import (
	"net/http"

	"github.com/immerle/immerle/internal/models"
)

// handleLibraryStats returns the cached library analytics: catalog counts plus
// the aggregate on-disk size and duration. The snapshot is computed at each scan
// (not per request), so this is a cheap in-memory read.
//
// @Summary      Library analytics
// @Description  Returns library-wide analytics: artist/album/track counts, total on-disk size (bytes) and total duration (seconds). Cached and refreshed at each scan.
// @Tags         library
// @Produce      json
// @Param        u  query  string  true   "Subsonic username"
// @Param        p  query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c  query  string  true   "Client name"
// @Success      200  {object}  LibraryStatsResponse
// @Router       /library/stats [get]
func (h *Handler) handleLibraryStats(w http.ResponseWriter, r *http.Request) {
	if h.LibraryStats == nil {
		writeJSON(w, http.StatusOK, okBody(map[string]any{"stats": models.LibraryStats{}}))
		return
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"stats": h.LibraryStats.Get()}))
}
