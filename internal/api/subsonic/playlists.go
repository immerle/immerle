package subsonic

import (
	"net/http"
	"strconv"
	"time"

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
	user := userFrom(r.Context())
	lists, err := h.Playlists.ListVisible(r.Context(), user.ID)
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
	p, err := h.Playlists.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "Playlist not found")
		return
	}
	user := userFrom(r.Context())
	if !h.canViewPlaylist(r, p, user) {
		writeError(w, r, ErrUnauthorizedAction, "Not authorized")
		return
	}
	tracks, err := h.Playlists.Tracks(r.Context(), id)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	trackAnn, _ := h.Annotations.AnnotationMap(r.Context(), user.ID, models.ItemTrack)
	entries := make([]Child, 0, len(tracks))
	for _, t := range tracks {
		entries = append(entries, toChild(t, annPtr(trackAnn, t.ID)))
	}
	resp := newResponse()
	out := toPlaylist(p, entries)
	resp.Playlist = &out
	write(w, r, resp)
}

func (h *Handler) handleCreatePlaylist(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	name := param(r, "name")
	playlistID := param(r, "playlistId")
	songIDs := r.Form["songId"]

	// updating an existing playlist when playlistId is provided
	if playlistID != "" {
		p, err := h.Playlists.Get(r.Context(), playlistID)
		if err != nil {
			writeError(w, r, ErrDataNotFound, "Playlist not found")
			return
		}
		if !h.canEditPlaylist(r, p, user) {
			writeError(w, r, ErrUnauthorizedAction, "Not authorized")
			return
		}
		if err := h.Playlists.ReplaceTracks(r.Context(), playlistID, songIDs, user.ID); err != nil {
			h.failInternal(w, r, err)
			return
		}
		h.respondPlaylist(w, r, playlistID)
		return
	}

	if name == "" {
		writeError(w, r, ErrMissingParameter, "Required parameter name is missing")
		return
	}
	now := time.Now()
	p := models.Playlist{
		ID:        newID(),
		Name:      name,
		OwnerID:   user.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.Playlists.Create(r.Context(), p); err != nil {
		h.failInternal(w, r, err)
		return
	}
	if len(songIDs) > 0 {
		_ = h.Playlists.ReplaceTracks(r.Context(), p.ID, songIDs, user.ID)
	}
	if h.Activity != nil {
		_ = h.Activity.Record(r.Context(), user, "add", models.ItemPlaylist, p.ID)
	}
	h.respondPlaylist(w, r, p.ID)
}

func (h *Handler) handleUpdatePlaylist(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := param(r, "playlistId")
	p, err := h.Playlists.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "Playlist not found")
		return
	}
	if !h.canEditPlaylist(r, p, user) {
		writeError(w, r, ErrUnauthorizedAction, "Not authorized")
		return
	}

	if v := param(r, "name"); v != "" {
		p.Name = v
	}
	if _, ok := r.Form["comment"]; ok {
		p.Comment = param(r, "comment")
	}
	if _, ok := r.Form["public"]; ok {
		p.Public = boolParam(r, "public", p.Public)
	}
	if err := h.Playlists.UpdateMeta(r.Context(), p); err != nil {
		h.failInternal(w, r, err)
		return
	}

	// Add songs (append, collaborative-safe) and remove by index.
	if add := r.Form["songIdToAdd"]; len(add) > 0 {
		_ = h.Playlists.AppendTracks(r.Context(), id, add, user.ID)
	}
	if rem := r.Form["songIndexToRemove"]; len(rem) > 0 {
		idxs := make([]int, 0, len(rem))
		for _, s := range rem {
			idxs = append(idxs, atoiSafe(s))
		}
		_ = h.Playlists.RemoveIndexes(r.Context(), id, idxs)
	}
	writeOK(w, r)
}

func (h *Handler) handleDeletePlaylist(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := param(r, "id")
	p, err := h.Playlists.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "Playlist not found")
		return
	}
	if p.OwnerID != user.ID && !user.IsAdmin {
		// A non-owner "deleting" the playlist just removes it from their library
		// (unsubscribe); it is not theirs to delete.
		if ok, _ := h.Playlists.Unsubscribe(r.Context(), id, user.ID); ok {
			writeOK(w, r)
			return
		}
		writeError(w, r, ErrUnauthorizedAction, "Not authorized")
		return
	}
	if err := h.Playlists.Delete(r.Context(), id); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}

func (h *Handler) respondPlaylist(w http.ResponseWriter, r *http.Request, id string) {
	p, err := h.Playlists.Get(r.Context(), id)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	tracks, _ := h.Playlists.Tracks(r.Context(), id)
	entries := make([]Child, 0, len(tracks))
	for _, t := range tracks {
		entries = append(entries, toChild(t, nil))
	}
	resp := newResponse()
	out := toPlaylist(p, entries)
	resp.Playlist = &out
	write(w, r, resp)
}

func (h *Handler) canViewPlaylist(r *http.Request, p models.Playlist, user models.User) bool {
	if p.OwnerID == user.ID || p.Public || user.IsAdmin {
		return true
	}
	collab, _ := h.Playlists.IsCollaborator(r.Context(), p.ID, user.ID)
	return collab
}

func (h *Handler) canEditPlaylist(r *http.Request, p models.Playlist, user models.User) bool {
	if p.Federated {
		return false // federated playlists are read-only
	}
	if p.OwnerID == user.ID || user.IsAdmin {
		return true
	}
	if p.Collaborative {
		collab, _ := h.Playlists.IsCollaborator(r.Context(), p.ID, user.ID)
		return collab
	}
	return false
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
