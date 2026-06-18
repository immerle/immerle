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
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  LibraryStatsDTO
// @Failure      401  {object}  errorResponse
// @Router       /library/stats [get]
func (h *Handler) handleLibraryStats(w http.ResponseWriter, r *http.Request) {
	if h.LibraryStats == nil {
		writeResource(w, http.StatusOK, models.LibraryStats{})
		return
	}
	writeResource(w, http.StatusOK, h.LibraryStats.Get())
}
