package subsonic

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

func (h *Handler) handleGetMusicFolders(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	folders := make([]MusicFolder, 0, len(h.MusicFolderPaths))
	for i, p := range h.MusicFolderPaths {
		name := p
		if idx := strings.LastIndexAny(strings.TrimRight(p, "/"), "/"); idx >= 0 {
			name = p[idx+1:]
		}
		folders = append(folders, MusicFolder{ID: musicFolderID(i), Name: name})
	}
	resp.MusicFolders = &MusicFolders{MusicFolder: folders}
	write(w, r, resp)
}

func musicFolderID(i int) string {
	return "folder-" + strconv.Itoa(i)
}

func (h *Handler) handleGetIndexes(w http.ResponseWriter, r *http.Request) {
	artists, _, err := h.library.Artists(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	idx := &Indexes{IgnoredArticles: "The El La Los Las Le Les"}
	idx.Index = groupArtistsLegacy(artists)
	resp.Indexes = idx
	write(w, r, resp)
}

func (h *Handler) handleGetArtists(w http.ResponseWriter, r *http.Request) {
	artists, starred, err := h.library.Artists(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	out := &ArtistsID3{IgnoredArticles: "The El La Los Las Le Les"}
	out.Index = groupArtistsID3(artists, starred)
	resp.Artists = out
	write(w, r, resp)
}

func (h *Handler) handleGetArtist(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	res, err := h.library.GetArtist(r.Context(), user.ID, param(r, "id"), boolParam(r, "includeSongs", false))
	if err != nil {
		h.writeServiceError(w, r, err, "Artist not found")
		return
	}
	resp := newResponse()
	out := toArtistID3(res.Artist, res.Annotation, albumEntriesToID3(res.Albums))
	resp.Artist = &out
	write(w, r, resp)
}

func (h *Handler) handleGetAlbum(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	res, err := h.library.GetAlbum(r.Context(), user.ID, param(r, "id"))
	if err != nil {
		h.writeServiceError(w, r, err, "Album not found")
		return
	}
	resp := newResponse()
	out := toAlbumID3(res.Album, res.Annotation, trackEntriesToChildren(res.Tracks))
	resp.Album = &out
	write(w, r, resp)
}

// albumEntriesToID3 renders library album entries as Subsonic AlbumID3, inlining
// songs when the entry carries them.
func albumEntriesToID3(entries []core.AlbumEntry) []AlbumID3 {
	out := make([]AlbumID3, 0, len(entries))
	for _, e := range entries {
		out = append(out, toAlbumID3(e.Album, e.Annotation, trackEntriesToChildren(e.Tracks)))
	}
	return out
}

// trackEntriesToChildren renders track entries as Subsonic children. It returns
// nil for an empty list so the album's Song element stays absent.
func trackEntriesToChildren(entries []core.TrackEntry) []Child {
	if len(entries) == 0 {
		return nil
	}
	out := make([]Child, 0, len(entries))
	for _, e := range entries {
		out = append(out, toChild(e.Track, e.Annotation))
	}
	return out
}

func (h *Handler) handleGetAlbumList2(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	albums, err := h.library.AlbumList(r.Context(), buildAlbumListOptions(r, user.ID))
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	resp.AlbumList2 = &AlbumList2{Album: albumEntriesToID3(albums)}
	write(w, r, resp)
}

func (h *Handler) handleGetSong(w http.ResponseWriter, r *http.Request) {
	te, err := h.library.Song(r.Context(), userFrom(r.Context()).ID, param(r, "id"))
	if err != nil {
		h.writeServiceError(w, r, err, "Song not found")
		return
	}
	resp := newResponse()
	child := toChild(te.Track, te.Annotation)
	resp.Song = &child
	write(w, r, resp)
}

func (h *Handler) handleGetGenres(w http.ResponseWriter, r *http.Request) {
	genres, err := h.Genres.List(r.Context())
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	out := &Genres{}
	for _, g := range genres {
		out.Genre = append(out.Genre, Genre{SongCount: g.SongCount, AlbumCount: g.AlbumCount, Name: g.Name})
	}
	resp.Genres = out
	write(w, r, resp)
}

func (h *Handler) handleGetStarred2(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	st := h.library.Starred(r.Context(), user.ID)
	out := &Starred2{}
	for _, a := range st.Artists {
		out.Artist = append(out.Artist, toArtistID3(a, nil, nil))
	}
	for _, a := range st.Albums {
		out.Album = append(out.Album, toAlbumID3(a, nil, nil))
	}
	for _, t := range st.Songs {
		out.Song = append(out.Song, toChild(t, nil))
	}
	resp := newResponse()
	resp.Starred2 = out
	write(w, r, resp)
}

// ---- helpers ----

func annPtr(m map[string]models.Annotation, id string) *models.Annotation {
	if a, ok := m[id]; ok {
		return &a
	}
	return nil
}

func buildAlbumListOptions(r *http.Request, userID string) persistence.AlbumListOptions {
	return persistence.AlbumListOptions{
		Type:     param(r, "type"),
		Size:     intParam(r, "size", 10),
		Offset:   intParam(r, "offset", 0),
		Genre:    param(r, "genre"),
		FromYear: intParam(r, "fromYear", 0),
		ToYear:   intParam(r, "toYear", 0),
		UserID:   userID,
	}
}

func indexLetter(name string) string {
	name = stripArticles(name)
	for _, ru := range name {
		if unicode.IsLetter(ru) {
			return strings.ToUpper(string(ru))
		}
		if unicode.IsDigit(ru) {
			return "#"
		}
	}
	return "#"
}

func stripArticles(name string) string {
	lower := strings.ToLower(name)
	for _, art := range []string{"the ", "a ", "an ", "le ", "la ", "les "} {
		if strings.HasPrefix(lower, art) {
			return name[len(art):]
		}
	}
	return name
}

func groupArtistsID3(artists []models.Artist, starred map[string]models.Annotation) []IndexID3 {
	buckets := map[string][]ArtistID3{}
	for _, a := range artists {
		letter := indexLetter(a.Name)
		buckets[letter] = append(buckets[letter], toArtistID3(a, annPtr(starred, a.ID), nil))
	}
	return sortedIndexID3(buckets)
}

func groupArtistsLegacy(artists []models.Artist) []Index {
	buckets := map[string][]ArtistItem{}
	for _, a := range artists {
		letter := indexLetter(a.Name)
		buckets[letter] = append(buckets[letter], ArtistItem{ID: a.ID, Name: a.Name})
	}
	return sortedIndex(buckets, func(l string, items []ArtistItem) Index {
		return Index{Name: l, Artist: items}
	})
}

func sortedIndexID3(buckets map[string][]ArtistID3) []IndexID3 {
	return sortedIndex(buckets, func(l string, items []ArtistID3) IndexID3 {
		return IndexID3{Name: l, Artist: items}
	})
}

// sortedIndex emits an alphabetically-sorted index list from letter→items
// buckets, building each entry with mk.
func sortedIndex[V, R any](buckets map[string][]V, mk func(letter string, items []V) R) []R {
	letters := make([]string, 0, len(buckets))
	for k := range buckets {
		letters = append(letters, k)
	}
	sort.Strings(letters)
	out := make([]R, 0, len(letters))
	for _, l := range letters {
		out = append(out, mk(l, buckets[l]))
	}
	return out
}
