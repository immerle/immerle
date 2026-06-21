package immerle

import (
	"net/http"
)

// handlePublicPlaylists lists public playlists the caller can subscribe to.
//
// @Summary      Browse public playlists
// @Description  Lists public playlists (not owned by the caller) available to subscribe to. Each entry includes whether the caller is already subscribed.
// @Tags         playlists
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  PublicPlaylistDTO
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /playlists/public [get]
func (h *Handler) handlePublicPlaylists(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	lists, err := h.Playlists.ListPublic(r.Context(), user.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]map[string]any, 0, len(lists))
	for _, p := range lists {
		subscribed, _ := h.Playlists.IsSubscribed(r.Context(), p.ID, user.ID)
		out = append(out, map[string]any{
			"id":         p.ID,
			"name":       p.Name,
			"owner":      p.OwnerName,
			"comment":    p.Comment,
			"songCount":  p.SongCount,
			"duration":   p.Duration,
			"coverArt":   p.CoverArt,
			"coverArts":  p.CoverArts,
			"subscribed": subscribed,
		})
	}
	writeResource(w, http.StatusOK, out)
}

// handleSubscribePlaylist subscribes the caller to a public playlist.
//
// @Summary      Subscribe to a public playlist
// @Description  Adds a public playlist to the caller's library (read-only). It then appears in getPlaylists like a normal playlist. Idempotent.
// @Tags         playlists
// @Security     BearerAuth
// @Param        id  path  string  true  "Playlist id to subscribe to"
// @Success      204  "subscribed"
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /playlists/{id}/subscription [put]
func (h *Handler) handleSubscribePlaylist(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := pathParam(r, "id")
	p, err := h.Playlists.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "playlist not found")
		return
	}
	// Only public playlists are subscribable; the owner needn't subscribe.
	if !p.Public || p.OwnerID == user.ID {
		writeError(w, http.StatusForbidden, "forbidden", "playlist is not public")
		return
	}
	if err := h.Playlists.Subscribe(r.Context(), id, user.ID); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// handleUnsubscribePlaylist removes a subscription.
//
// @Summary      Unsubscribe from a playlist
// @Tags         playlists
// @Security     BearerAuth
// @Param        id  path  string  true  "Playlist id"
// @Success      204  "unsubscribed"
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /playlists/{id}/subscription [delete]
func (h *Handler) handleUnsubscribePlaylist(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if _, err := h.Playlists.Unsubscribe(r.Context(), pathParam(r, "id"), user.ID); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}
