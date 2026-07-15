package immerle

import (
	"context"
	"net/http"
	"time"

	"github.com/immerle/immerle/internal/models"
)

// This file exposes the personal "discovery" lists — computed live from the
// caller's scrobble/annotation history, never stored (same principle as
// Wrapped) — as opposed to the genre/decade auto-playlists (internal/autoplaylists),
// which are real, shared Playlist rows.

// discoverWindow resolves the [start, end) unix-millis range for handleTopTracks's
// `window` param: "month" (the current calendar month, UTC — same per-month
// window Wrapped's ByMonth histogram buckets by, just narrowed to one month)
// or "30d" (a rolling last-30-days window, for "on repeat").
func discoverWindow(window string) (start, end int64) {
	now := time.Now().UTC()
	if window == "30d" {
		return now.AddDate(0, 0, -30).UnixMilli(), now.UnixMilli()
	}
	s := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return s.UnixMilli(), s.AddDate(0, 1, 0).UnixMilli()
}

// handleTopTracks returns the caller's most-played tracks in a window: the
// current calendar month (default) or a rolling last 30 days (?window=30d) —
// "Top of your month" / "On Repeat" are the same query, just a different window.
//
// @Summary      Personal top tracks
// @Description  Returns the caller's most-played tracks in a time window: the current calendar month (default) or a rolling last 30 days (?window=30d).
// @Tags         catalog
// @Security     BearerAuth
// @Produce      json
// @Param        window  query  string  false  "month (default) or 30d"
// @Param        limit   query  int     false  "Max tracks"  default(20)
// @Success      200  {object}  map[string][]songView
// @Failure      401  {object}  errorResponse
// @Router       /me/top-tracks [get]
func (h *Handler) handleTopTracks(w http.ResponseWriter, r *http.Request) {
	if h.Wrapped == nil {
		writeResource(w, http.StatusOK, map[string]any{"songs": []songView{}})
		return
	}
	userID := userFrom(r.Context()).ID
	start, end := discoverWindow(r.URL.Query().Get("window"))

	top, err := h.Wrapped.TopTracks(r.Context(), userID, start, end, intQuery(r, "limit", 20))
	if err != nil {
		writeInternal(w, err)
		return
	}
	ids := make([]string, len(top))
	for i, t := range top {
		ids[i] = t.ID
	}
	out, err := h.orderedSongViews(r.Context(), userID, ids)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, map[string]any{"songs": out})
}

// handleForgottenFavorites returns the caller's starred tracks not played in
// at least ?minDays (default 90) — including never-played ones — most
// recently starred first: favorites worth resurfacing.
//
// @Summary      Forgotten favorites
// @Description  Returns the caller's starred tracks that haven't been played in at least minDays (default 90), or never played at all.
// @Tags         catalog
// @Security     BearerAuth
// @Produce      json
// @Param        minDays  query  int  false  "Minimum days since last play"  default(90)
// @Param        limit    query  int  false  "Max tracks"  default(20)
// @Success      200  {object}  map[string][]songView
// @Failure      401  {object}  errorResponse
// @Router       /me/forgotten-favorites [get]
func (h *Handler) handleForgottenFavorites(w http.ResponseWriter, r *http.Request) {
	userID := userFrom(r.Context()).ID
	cutoff := time.Now().AddDate(0, 0, -intQuery(r, "minDays", 90))

	ids, err := h.Annotations.ForgottenFavorites(r.Context(), userID, models.ItemTrack, cutoff, intQuery(r, "limit", 20))
	if err != nil {
		writeInternal(w, err)
		return
	}
	out, err := h.orderedSongViews(r.Context(), userID, ids)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, map[string]any{"songs": out})
}

// orderedSongViews batch-fetches tracks by id (preserving the given order,
// unlike CatalogRepo.TracksByIDs' map) and renders them with the caller's
// annotations attached. A shared helper since both discovery lists above are
// "id list in relevance order" + "look up and view."
func (h *Handler) orderedSongViews(ctx context.Context, userID string, ids []string) ([]songView, error) {
	byID, err := h.Catalog.TracksByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	ann, err := h.Annotations.AnnotationMap(ctx, userID, models.ItemTrack)
	if err != nil {
		return nil, err
	}
	out := make([]songView, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			out = append(out, toSongViewAnnotated(t, annPtr(ann, id)))
		}
	}
	return out, nil
}
