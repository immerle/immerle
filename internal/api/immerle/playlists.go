package immerle

import (
	"net/http"
)

// handlePublicPlaylists lists public playlists the caller can subscribe to.
//
// @Summary      Browse public playlists
// @Description  Lists public playlists (not owned by the caller) available to subscribe to. Each entry includes whether the caller is already subscribed.
// @Tags         playlists
// @Produce      json
// @Param        u  query  string  true   "Subsonic username"
// @Param        p  query  string  false  "Subsonic password (or token auth)"
// @Param        c  query  string  true   "Client name"
// @Success      200  {object}  PublicPlaylistsResponse
// @Router       /playlists/public [get]
func (h *Handler) handlePublicPlaylists(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	lists, err := h.Playlists.ListPublic(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
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
			"coverArts":  p.CoverArts,
			"subscribed": subscribed,
		})
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"playlists": out}))
}

// handleSubscribePlaylist subscribes the caller to a public playlist.
//
// @Summary      Subscribe to a public playlist
// @Description  Adds a public playlist to the caller's library (read-only). It then appears in getPlaylists like a normal playlist.
// @Tags         playlists
// @Produce      json
// @Param        u           query  string  true   "Subsonic username"
// @Param        p           query  string  false  "Subsonic password (or token auth)"
// @Param        c           query  string  true   "Client name"
// @Param        playlistId  query  string  true   "Playlist id to subscribe to"
// @Success      200  {object}  OKResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /playlists/subscribe [post]
func (h *Handler) handleSubscribePlaylist(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := r.Form.Get("playlistId")
	p, err := h.Playlists.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody("playlist not found"))
		return
	}
	// Only public playlists are subscribable; the owner needn't subscribe.
	if !p.Public || p.OwnerID == user.ID {
		writeJSON(w, http.StatusForbidden, errorBody("playlist is not public"))
		return
	}
	if err := h.Playlists.Subscribe(r.Context(), id, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, okBody(nil))
}

// handleUnsubscribePlaylist removes a subscription.
//
// @Summary      Unsubscribe from a playlist
// @Tags         playlists
// @Produce      json
// @Param        u           query  string  true   "Subsonic username"
// @Param        p           query  string  false  "Subsonic password (or token auth)"
// @Param        c           query  string  true   "Client name"
// @Param        playlistId  query  string  true   "Playlist id"
// @Success      200  {object}  OKResponse
// @Router       /playlists/unsubscribe [post]
func (h *Handler) handleUnsubscribePlaylist(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if _, err := h.Playlists.Unsubscribe(r.Context(), r.Form.Get("playlistId"), user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, okBody(nil))
}
