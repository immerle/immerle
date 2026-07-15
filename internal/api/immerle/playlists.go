package immerle

import (
	"net/http"

	"github.com/immerle/immerle/internal/autoplaylists"
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

// customPlaylistSources are looked up directly by (kind, callerID), in
// display order.
var customPlaylistSources = []string{autoplaylists.SourceTopMonth, autoplaylists.SourceOnRepeat, autoplaylists.SourceForgotten}

// handleCustomPlaylists returns the caller's auto-generated personal
// playlists ("Top du mois", "On Repeat", "Favoris oubliés") that currently
// have at least one track. Looked up directly by (kind, callerID) — not
// through ListVisible/subscriptions — so unsubscribing/unliking one (easy to
// do by mistake, since a federated playlist hides normal owner controls)
// never loses access to it.
//
// @Summary      Custom auto-generated playlists
// @Description  Returns the caller's personal auto-generated playlists (top of the month, on repeat, forgotten favorites) that currently have at least one track.
// @Tags         playlists
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string][]playlistView
// @Failure      401  {object}  errorResponse
// @Router       /me/custom-playlists [get]
func (h *Handler) handleCustomPlaylists(w http.ResponseWriter, r *http.Request) {
	userID := userFrom(r.Context()).ID
	out := make([]playlistView, 0, len(customPlaylistSources))
	for _, source := range customPlaylistSources {
		p, err := h.Playlists.FindFederated(r.Context(), source, userID)
		if err != nil || p.SongCount == 0 {
			continue
		}
		out = append(out, toPlaylistView(p, nil))
	}
	writeResource(w, http.StatusOK, map[string]any{"playlists": out})
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
	// Only public playlists are subscribable; the real owner needn't subscribe —
	// but a federated playlist's "owner" is only an internal attribution (see
	// ListVisible), so it must stay subscribable even for that account.
	if !p.Public || (p.OwnerID == user.ID && !p.Federated) {
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
