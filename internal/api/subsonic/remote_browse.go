package subsonic

import (
	"context"
	"net/http"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
)

// localAnn resolves a track's annotation (like/rating/play). When the id is a
// remote (provider) track that has already been downloaded, its state is taken
// from the local copy — so a liked title shows as liked even while listed under
// its remote id on a provider album/artist page.
func (h *Handler) localAnn(ctx context.Context, trackAnn map[string]models.Annotation, id string) *models.Annotation {
	if a := annPtr(trackAnn, id); a != nil {
		return a
	}
	if h.OnDemand != nil && core.IsRemoteID(id) {
		if localID, ok := h.OnDemand.LocalTrackIDForRemote(ctx, id); ok {
			return annPtr(trackAnn, localID)
		}
	}
	return nil
}

// remoteSongs converts provider tracks to Child entries with resolved like state.
func (h *Handler) remoteSongs(r *http.Request, tracks []models.Track) []Child {
	ctx := r.Context()
	trackAnn, _ := h.Annotations.AnnotationMap(ctx, userFrom(ctx).ID, models.ItemTrack)
	songs := make([]Child, 0, len(tracks))
	for _, t := range tracks {
		songs = append(songs, toChild(t, h.localAnn(ctx, trackAnn, t.ID)))
	}
	return songs
}

// remoteMusicDirectory serves getMusicDirectory for a remote artist (albums as
// sub-directories) or a remote album (songs).
func (h *Handler) remoteMusicDirectory(w http.ResponseWriter, r *http.Request, id string) bool {
	switch {
	case core.IsRemoteArtistID(id):
		artist, albums, err := h.OnDemand.RemoteArtist(r.Context(), id)
		if err != nil || artist.Name == "" {
			writeError(w, r, ErrDataNotFound, "Directory not found")
			return true
		}
		children := make([]Child, 0, len(albums))
		for _, a := range albums {
			children = append(children, toAlbumChild(a, nil))
		}
		resp := newResponse()
		resp.Directory = &Directory{ID: artist.ID, Name: artist.Name, Child: children}
		write(w, r, resp)
		return true
	case core.IsRemoteAlbumID(id):
		album, tracks, err := h.OnDemand.RemoteAlbum(r.Context(), id)
		if err != nil || album.Name == "" {
			writeError(w, r, ErrDataNotFound, "Directory not found")
			return true
		}
		songs := h.remoteSongs(r, tracks)
		resp := newResponse()
		resp.Directory = &Directory{ID: album.ID, Name: album.Name, Parent: album.ArtistID, Child: songs}
		write(w, r, resp)
		return true
	}
	return false
}

// remotePlaylist serves getPlaylist for a remote (provider) playlist id
// surfaced in search; it does not exist as a local playlist row.
func (h *Handler) remotePlaylist(w http.ResponseWriter, r *http.Request, id string) bool {
	if !core.IsRemotePlaylistID(id) {
		return false
	}
	pl, err := h.OnDemand.RemotePlaylist(r.Context(), id)
	if err != nil || pl.Name == "" {
		writeError(w, r, ErrDataNotFound, "Playlist not found")
		return true
	}
	ctx := r.Context()
	trackAnn, _ := h.Annotations.AnnotationMap(ctx, userFrom(ctx).ID, models.ItemTrack)
	tracks := make([]core.TrackEntry, 0, len(pl.Tracks))
	for _, t := range pl.Tracks {
		tracks = append(tracks, core.TrackEntry{Track: t, Annotation: h.localAnn(ctx, trackAnn, t.ID)})
	}
	h.writePlaylist(w, r, core.PlaylistDetail{Playlist: pl, Tracks: tracks})
	return true
}
