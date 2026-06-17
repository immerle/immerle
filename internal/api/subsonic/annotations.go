package subsonic

import (
	"context"
	"net/http"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
)

func (h *Handler) handleStar(w http.ResponseWriter, r *http.Request) {
	h.setStar(w, r, true)
}

func (h *Handler) handleUnstar(w http.ResponseWriter, r *http.Request) {
	h.setStar(w, r, false)
}

// localTrackID maps a possibly-remote (provider) track id to its local track id,
// downloading the track on demand if needed. Annotations (stars, ratings, plays)
// must attach to the local track, otherwise they target a synthetic remote id
// that no library row matches — so the star would never show in favorites and
// would vanish on the next search. Non-remote ids (and failures) pass through.
func (h *Handler) localTrackID(ctx context.Context, userID, id string) string {
	if core.IsRemoteID(id) && h.OnDemand != nil {
		if track, _, _, err := h.OnDemand.Resolve(ctx, userID, id); err == nil && track.ID != "" {
			return track.ID
		}
	}
	return id
}

// setStar applies (un)starring to the id/albumId/artistId parameters.
func (h *Handler) setStar(w http.ResponseWriter, r *http.Request, star bool) {
	user := userFrom(r.Context())
	ctx := r.Context()

	for _, rawID := range r.Form["id"] {
		id := h.localTrackID(ctx, user.ID, rawID)
		_ = h.Annotations.SetStarred(ctx, user.ID, models.ItemTrack, id, star)
		if star && h.Activity != nil {
			_ = h.Activity.Record(ctx, user, "favorite", models.ItemTrack, id)
		}
	}
	for _, id := range r.Form["albumId"] {
		_ = h.Annotations.SetStarred(ctx, user.ID, models.ItemAlbum, id, star)
		if star && h.Activity != nil {
			_ = h.Activity.Record(ctx, user, "favorite", models.ItemAlbum, id)
		}
	}
	for _, id := range r.Form["artistId"] {
		_ = h.Annotations.SetStarred(ctx, user.ID, models.ItemArtist, id, star)
	}
	writeOK(w, r)
}

func (h *Handler) handleSetRating(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := param(r, "id")
	if id == "" {
		writeError(w, r, ErrMissingParameter, "Required parameter id is missing")
		return
	}
	rating := intParam(r, "rating", 0)

	// A remote (provider) track id is rated on its local copy (downloaded if
	// needed), like stars — otherwise the rating orphans on a synthetic id.
	if core.IsRemoteID(id) {
		id = h.localTrackID(r.Context(), userFrom(r.Context()).ID, id)
		if err := h.Annotations.SetRating(r.Context(), user.ID, models.ItemTrack, id, rating); err != nil {
			h.failInternal(w, r, err)
			return
		}
		writeOK(w, r)
		return
	}

	// Rate whichever item type the id refers to.
	itemType := models.ItemTrack
	if _, err := h.Catalog.GetAlbum(r.Context(), id); err == nil {
		itemType = models.ItemAlbum
	} else if _, err := h.Catalog.GetArtist(r.Context(), id); err == nil {
		itemType = models.ItemArtist
	}
	if err := h.Annotations.SetRating(r.Context(), user.ID, itemType, id, rating); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}
