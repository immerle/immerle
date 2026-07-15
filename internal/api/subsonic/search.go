package subsonic

import (
	"net/http"
)

func (h *Handler) handleSearch3(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	res, err := h.library.Search(r.Context(), user.ID, param(r, "query"),
		intParam(r, "artistCount", 20), intParam(r, "albumCount", 20), intParam(r, "songCount", 20), 0)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}

	out := &SearchResult3{}
	for _, a := range res.Artists {
		out.Artist = append(out.Artist, toArtistID3(a, nil, nil))
	}
	for _, a := range res.Albums {
		out.Album = append(out.Album, toAlbumID3(a, annPtr(res.AlbumAnnotations, a.ID), nil))
	}
	for _, t := range res.Tracks {
		out.Song = append(out.Song, toChild(t, annPtr(res.TrackAnnotations, t.ID)))
	}

	resp := newResponse()
	resp.SearchResult3 = out
	write(w, r, resp)
}
