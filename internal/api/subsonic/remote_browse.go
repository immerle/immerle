package subsonic

import (
	"context"
	"net/http"

	"github.com/gossignol/gossignol/internal/core"
	"github.com/gossignol/gossignol/internal/models"
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

// respondRemoteArtist renders a provider (remote) artist surfaced in search,
// with its albums grouped from the provider's tracks.
func (h *Handler) respondRemoteArtist(w http.ResponseWriter, r *http.Request, id string) {
	artist, albums, err := h.OnDemand.RemoteArtist(r.Context(), id)
	if err != nil || artist.Name == "" {
		writeError(w, r, ErrDataNotFound, "Artist not found")
		return
	}
	albumList := make([]AlbumID3, 0, len(albums))
	for _, a := range albums {
		albumList = append(albumList, toAlbumID3(a, nil, nil))
	}
	resp := newResponse()
	out := toArtistID3(artist, nil, albumList)
	resp.Artist = &out
	write(w, r, resp)
}

// respondRemoteAlbum renders a provider (remote) album with its tracks.
func (h *Handler) respondRemoteAlbum(w http.ResponseWriter, r *http.Request, id string) {
	album, tracks, err := h.OnDemand.RemoteAlbum(r.Context(), id)
	if err != nil || album.Name == "" {
		writeError(w, r, ErrDataNotFound, "Album not found")
		return
	}
	songs := h.remoteSongs(r, tracks)
	resp := newResponse()
	out := toAlbumID3(album, nil, songs)
	resp.Album = &out
	write(w, r, resp)
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
