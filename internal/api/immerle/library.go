package immerle

import (
	"context"
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
// @Success      200  {object}  models.LibraryStats
// @Failure      401  {object}  errorResponse
// @Router       /library/stats [get]
func (h *Handler) handleLibraryStats(w http.ResponseWriter, r *http.Request) {
	if h.LibraryStats == nil {
		writeResource(w, http.StatusOK, models.LibraryStats{})
		return
	}
	writeResource(w, http.StatusOK, h.LibraryStats.Get())
}

// scanStatusView is the library scan state.
type scanStatusView struct {
	Scanning bool `json:"scanning"`
	// Count is the number of tracks indexed so far (it grows during a scan).
	Count int `json:"count"`
}

func (h *Handler) scanStatus(r *http.Request) (scanStatusView, error) {
	_, _, tracks, err := h.Catalog.Stats(r.Context())
	if err != nil {
		return scanStatusView{}, err
	}
	return scanStatusView{Scanning: h.Scanner != nil && h.Scanner.Scanning(), Count: tracks}, nil
}

// handleScanStatus reports whether a library scan is running.
//
// @Summary      Library scan state
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  scanStatusView
// @Failure      403  {object}  errorResponse
// @Router       /admin/library/scan [get]
func (h *Handler) handleScanStatus(w http.ResponseWriter, r *http.Request) {
	st, err := h.scanStatus(r)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, st)
}

// handleStartScan starts a library scan in the background. Every scan walks all
// the library roots (it re-reads each file); starting one while another runs
// is a no-op.
//
// @Summary      Start a library scan
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      202  {object}  scanStatusView
// @Failure      403  {object}  errorResponse
// @Failure      503  {object}  errorResponse  "no scanner or no library path"
// @Router       /admin/library/scan [post]
func (h *Handler) handleStartScan(w http.ResponseWriter, r *http.Request) {
	if h.Scanner == nil || len(h.LibraryPaths) == 0 {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "library scanning is not configured")
		return
	}
	go func() {
		// The scan outlives this request.
		if _, err := h.Scanner.ScanPaths(context.Background(), h.LibraryPaths); err != nil {
			h.Logger.Warn("admin-triggered scan failed", "error", err)
		}
	}()
	h.Logger.Info("library scan started", "by", userFrom(r.Context()).Username)
	st, err := h.scanStatus(r)
	if err != nil {
		writeInternal(w, err)
		return
	}
	st.Scanning = true // the goroutine may not have flipped the flag yet
	writeResource(w, http.StatusAccepted, st)
}
