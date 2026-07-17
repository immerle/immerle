package subsonic

import (
	"net/http"
	"strconv"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
)

func toPlaylist(p models.Playlist, entries []Child) Playlist {
	return Playlist{
		ID:        p.ID,
		Name:      p.Name,
		Comment:   p.Comment,
		Owner:     p.OwnerName,
		Public:    p.Public,
		SongCount: p.SongCount,
		Duration:  p.Duration,
		Created:   formatTime(p.CreatedAt),
		Changed:   formatTime(p.UpdatedAt),
		CoverArts: p.CoverArts,
		Entry:     entries,
	}
}

func (h *Handler) handleGetPlaylists(w http.ResponseWriter, r *http.Request) {
	lists, err := h.playlistSvc.List(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	out := &Playlists{}
	for _, p := range lists {
		out.Playlist = append(out.Playlist, toPlaylist(p, nil))
	}
	resp.Playlists = out
	write(w, r, resp)
}

func (h *Handler) handleGetPlaylist(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if h.OnDemand != nil && h.remotePlaylist(w, r, id) {
		return
	}
	d, err := h.playlistSvc.Get(r.Context(), userFrom(r.Context()), id)
	if err != nil {
		h.writeServiceError(w, r, err, "Playlist not found")
		return
	}
	h.writePlaylist(w, r, d)
}

func (h *Handler) handleCreatePlaylist(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	songIDs := r.Form["songId"]

	// With a playlistId this replaces an existing playlist's tracks.
	if playlistID := param(r, "playlistId"); playlistID != "" {
		d, err := h.playlistSvc.Replace(r.Context(), user, playlistID, songIDs)
		if err != nil {
			h.writeServiceError(w, r, err, "Playlist not found")
			return
		}
		h.writePlaylist(w, r, d)
		return
	}

	name := param(r, "name")
	if name == "" {
		writeError(w, r, ErrMissingParameter, "Required parameter name is missing")
		return
	}
	d, err := h.playlistSvc.Create(r.Context(), user, name, songIDs)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	h.writePlaylist(w, r, d)
}

func (h *Handler) handleUpdatePlaylist(w http.ResponseWriter, r *http.Request) {
	id := param(r, "playlistId")

	// Parse remove indexes up front; a malformed value is a client error.
	var removeIndexes []int
	if rem := r.Form["songIndexToRemove"]; len(rem) > 0 {
		removeIndexes = make([]int, 0, len(rem))
		for _, s := range rem {
			n, err := strconv.Atoi(s)
			if err != nil {
				writeError(w, r, ErrMissingParameter, "Invalid songIndexToRemove")
				return
			}
			removeIndexes = append(removeIndexes, n)
		}
	}

	meta := core.PlaylistMetaUpdate{Name: param(r, "name")}
	if _, ok := r.Form["comment"]; ok {
		c := param(r, "comment")
		meta.Comment = &c
	}
	if _, ok := r.Form["public"]; ok {
		p := param(r, "public")
		meta.PublicRaw = &p
	}

	if err := h.playlistSvc.Update(r.Context(), userFrom(r.Context()), id, meta, r.Form["songIdToAdd"], removeIndexes); err != nil {
		h.writeServiceError(w, r, err, "Playlist not found")
		return
	}
	writeOK(w, r)
}

func (h *Handler) handleDeletePlaylist(w http.ResponseWriter, r *http.Request) {
	if err := h.playlistSvc.Delete(r.Context(), userFrom(r.Context()), param(r, "id")); err != nil {
		h.writeServiceError(w, r, err, "Playlist not found")
		return
	}
	writeOK(w, r)
}

// writePlaylist renders a playlist detail response.
func (h *Handler) writePlaylist(w http.ResponseWriter, r *http.Request, d core.PlaylistDetail) {
	resp := newResponse()
	out := toPlaylist(d.Playlist, trackEntriesToChildren(d.Tracks))
	resp.Playlist = &out
	write(w, r, resp)
}
