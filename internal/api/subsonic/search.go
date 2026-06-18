package subsonic

import (
	"net/http"
	"sort"
	"strings"

	"github.com/immerle/immerle/internal/models"
)

// Final search result caps: titles and albums to 10, artists to 4. Applied to
// the merged local+remote lists after re-sorting by relevance.
const (
	maxSearchArtists = 4
	maxSearchAlbums  = 10
	maxSearchSongs   = 10
)

// relevance scores how well s matches the query for search ordering: exact (0),
// prefix (1), substring (2), otherwise (3). Lower is better; ties keep input
// order (stable sort).
func relevance(query, s string) int {
	q, x := strings.ToLower(strings.TrimSpace(query)), strings.ToLower(s)
	switch {
	case q == "" || x == q:
		return 0
	case strings.HasPrefix(x, q):
		return 1
	case strings.Contains(x, q):
		return 2
	default:
		return 3
	}
}

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

	// Remote results from every active provider, merged into the local lists
	// (deduplicated by name for artists/albums, id for songs).
	if h.OnDemand != nil && query != "" {
		remoteArtists, remoteAlbums, remoteSongs := h.OnDemand.RemoteSearch3(r.Context(), query, maxSearchArtists, maxSearchAlbums, maxSearchSongs)

		seenA := make(map[string]bool, len(out.Artist))
		for _, a := range out.Artist {
			seenA[strings.ToLower(a.Name)] = true
		}
		for _, a := range remoteArtists {
			if seenA[strings.ToLower(a.Name)] {
				continue
			}
			seenA[strings.ToLower(a.Name)] = true
			out.Artist = append(out.Artist, toArtistID3(a, nil, nil))
		}

		seenAl := make(map[string]bool, len(out.Album))
		for _, a := range out.Album {
			seenAl[strings.ToLower(a.Artist+"|"+a.Name)] = true
		}
		for _, a := range remoteAlbums {
			if seenAl[strings.ToLower(a.ArtistName+"|"+a.Name)] {
				continue
			}
			seenAl[strings.ToLower(a.ArtistName+"|"+a.Name)] = true
			out.Album = append(out.Album, toAlbumID3(a, nil, nil))
		}

		seenS := make(map[string]bool, len(out.Song))
		for _, s := range out.Song {
			seenS[s.ID] = true
		}
		for _, t := range remoteSongs {
			if seenS[t.ID] {
				continue
			}
			seenS[t.ID] = true
			out.Song = append(out.Song, toChild(t, nil))
		}
	}

	// Re-sort the merged lists by relevance to the query, then apply the caps.
	sort.SliceStable(out.Artist, func(i, j int) bool {
		return relevance(query, out.Artist[i].Name) < relevance(query, out.Artist[j].Name)
	})
	sort.SliceStable(out.Album, func(i, j int) bool {
		return relevance(query, out.Album[i].Name) < relevance(query, out.Album[j].Name)
	})
	sort.SliceStable(out.Song, func(i, j int) bool {
		return relevance(query, out.Song[i].Title) < relevance(query, out.Song[j].Title)
	})
	out.Artist = capSlice(out.Artist, maxSearchArtists)
	out.Album = capSlice(out.Album, maxSearchAlbums)
	out.Song = capSlice(out.Song, maxSearchSongs)

	resp := newResponse()
	resp.SearchResult3 = out
	write(w, r, resp)
}

// capSlice truncates s to at most n elements.
func capSlice[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}
