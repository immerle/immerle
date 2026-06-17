package subsonic

import (
	"net/http"
	"strings"
	"sync"

	"github.com/immerle/immerle/internal/models"
)

func (h *Handler) handleSearch3(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(param(r, "query"))
	// Subsonic clients sometimes quote queries or use "" to mean "everything".
	query = strings.Trim(query, "\"")

	artistCount := intParam(r, "artistCount", 20)
	albumCount := intParam(r, "albumCount", 20)
	songCount := intParam(r, "songCount", 20)

	artists, albums, tracks, err := h.Catalog.Search(r.Context(), query, artistCount, albumCount, songCount)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}

	user := userFrom(r.Context())
	trackAnn, _ := h.Annotations.AnnotationMap(r.Context(), user.ID, models.ItemTrack)
	albumAnn, _ := h.Annotations.AnnotationMap(r.Context(), user.ID, models.ItemAlbum)

	out := &SearchResult3{}
	for _, a := range artists {
		out.Artist = append(out.Artist, toArtistID3(a, nil, nil))
	}
	for _, a := range albums {
		out.Album = append(out.Album, toAlbumID3(a, annPtr(albumAnn, a.ID), nil))
	}
	for _, t := range tracks {
		out.Song = append(out.Song, toChild(t, annPtr(trackAnn, t.ID)))
	}

	// Remote songs and artists are fetched concurrently (one provider, but two
	// API calls for providers like Deezer) so the request isn't serialized.
	if h.OnDemand != nil && query != "" {
		var wg sync.WaitGroup
		var songs []Child
		var artists []ArtistID3
		wg.Add(2)
		go func() { defer wg.Done(); songs = h.mergeRemoteSongs(r, out.Song, query, songCount) }()
		go func() { defer wg.Done(); artists = h.mergeRemoteArtists(r, out.Artist, query, artistCount) }()
		wg.Wait()
		out.Song, out.Artist = songs, artists
	}

	resp := newResponse()
	resp.SearchResult3 = out
	write(w, r, resp)
}

// mergeRemoteArtists appends provider (remote) artists to the local artist list,
// deduplicated by name. Remote artists carry self-describing ids and are
// browsable via getArtist/getMusicDirectory.
func (h *Handler) mergeRemoteArtists(r *http.Request, local []ArtistID3, query string, artistCount int) []ArtistID3 {
	if h.OnDemand == nil || query == "" {
		return local
	}
	remote, err := h.OnDemand.RemoteSearchArtists(r.Context(), query, artistCount)
	if err != nil {
		return local
	}
	seen := make(map[string]bool, len(local))
	for _, a := range local {
		seen[strings.ToLower(a.Name)] = true
	}
	for _, a := range remote {
		if seen[strings.ToLower(a.Name)] {
			continue
		}
		seen[strings.ToLower(a.Name)] = true
		local = append(local, toArtistID3(a, nil, nil))
	}
	return local
}

// mergeRemoteSongs appends provider (remote) search results to the local song
// list, deduplicated by id. Remote results come from every registered provider
// (merged in remoteSearch); those from downloadable providers are streamable and
// trigger a background download on first play, while metadata-only providers
// (e.g. Deezer via ARL) surface the track for discovery.
func (h *Handler) mergeRemoteSongs(r *http.Request, local []Child, query string, songCount int) []Child {
	if h.OnDemand == nil || query == "" {
		return local
	}
	remote, err := h.OnDemand.RemoteSearch(r.Context(), query, songCount)
	if err != nil {
		return local
	}
	seen := make(map[string]bool, len(local))
	for _, s := range local {
		seen[s.ID] = true
	}
	for _, t := range remote {
		if seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		local = append(local, toChild(t, nil))
	}
	return local
}
