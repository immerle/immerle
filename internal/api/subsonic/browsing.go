package subsonic

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gossignol/gossignol/internal/core"
	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/persistence"
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
	return "folder-" + string(rune('0'+i))
}

func (h *Handler) handleGetIndexes(w http.ResponseWriter, r *http.Request) {
	artists, err := h.Catalog.ListArtists(r.Context())
	if err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	resp := newResponse()
	idx := &Indexes{IgnoredArticles: "The El La Los Las Le Les"}
	grouped := groupArtistsLegacy(artists)
	idx.Index = grouped
	resp.Indexes = idx
	write(w, r, resp)
}

func (h *Handler) handleGetArtists(w http.ResponseWriter, r *http.Request) {
	artists, err := h.Catalog.ListArtists(r.Context())
	if err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	user := userFrom(r.Context())
	starred, _ := h.Annotations.AnnotationMap(r.Context(), user.ID, models.ItemArtist)

	resp := newResponse()
	out := &ArtistsID3{IgnoredArticles: "The El La Los Las Le Les"}
	out.Index = groupArtistsID3(artists, starred)
	resp.Artists = out
	write(w, r, resp)
}

func (h *Handler) handleGetArtist(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if core.IsRemoteArtistID(id) && h.OnDemand != nil {
		h.respondRemoteArtist(w, r, id)
		return
	}
	artist, err := h.Catalog.GetArtist(r.Context(), id)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "Artist not found")
		return
	}
	albums, err := h.Catalog.ListAlbumsByArtist(r.Context(), id)
	if err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	user := userFrom(r.Context())
	albumAnn, _ := h.Annotations.AnnotationMap(r.Context(), user.ID, models.ItemAlbum)
	artistAnn, _ := h.Annotations.Get(r.Context(), user.ID, models.ItemArtist, id)

	albumList := make([]AlbumID3, 0, len(albums))
	seenAlbum := make(map[string]bool, len(albums))
	for _, a := range albums {
		ann := annPtr(albumAnn, a.ID)
		albumList = append(albumList, toAlbumID3(a, ann, nil))
		seenAlbum[strings.ToLower(a.Name)] = true
	}

	// Enrich with the rest of the artist's discography from the provider
	// (deduplicated against the local albums by name). These remote albums are
	// browsable and stream/download on play.
	if h.OnDemand != nil {
		if remote, err := h.OnDemand.RemoteAlbumsForArtist(r.Context(), artist.Name); err == nil {
			for _, ra := range remote {
				if seenAlbum[strings.ToLower(ra.Name)] {
					continue
				}
				seenAlbum[strings.ToLower(ra.Name)] = true
				albumList = append(albumList, toAlbumID3(ra, nil, nil))
			}
		}
	}

	artist.AlbumCount = len(albumList)

	// Optionally inline each album's songs (off by default to keep getArtist
	// light and Subsonic-standard).
	if boolParam(r, "includeSongs", false) {
		h.fillAlbumSongs(r, albumList)
	}

	resp := newResponse()
	out := toArtistID3(artist, &artistAnn, albumList)
	resp.Artist = &out
	write(w, r, resp)
}

// fillAlbumSongs populates AlbumID3.Song for each album: local albums from the
// catalog (cheap), remote albums from the provider (fetched concurrently, with
// a bounded concurrency and an overall timeout so it can't hang).
func (h *Handler) fillAlbumSongs(r *http.Request, albums []AlbumID3) {
	ctx := r.Context()
	user := userFrom(ctx)
	trackAnn, _ := h.Annotations.AnnotationMap(ctx, user.ID, models.ItemTrack)

	var remote []int
	for i := range albums {
		if core.IsRemoteAlbumID(albums[i].ID) {
			remote = append(remote, i)
			continue
		}
		tracks, err := h.Catalog.ListTracksByAlbum(ctx, albums[i].ID)
		if err != nil {
			continue
		}
		songs := make([]Child, 0, len(tracks))
		for _, t := range tracks {
			songs = append(songs, toChild(t, annPtr(trackAnn, t.ID)))
		}
		albums[i].Song = songs
	}

	if h.OnDemand == nil || len(remote) == 0 {
		return
	}
	rctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	sem := make(chan struct{}, 6) // bound concurrent provider calls
	var wg sync.WaitGroup
	for _, idx := range remote {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_, tracks, err := h.OnDemand.RemoteAlbum(rctx, albums[i].ID)
			if err != nil {
				return
			}
			songs := make([]Child, 0, len(tracks))
			for _, t := range tracks {
				songs = append(songs, toChild(t, h.localAnn(rctx, trackAnn, t.ID)))
			}
			albums[i].Song = songs // distinct index per goroutine — no race
		}(idx)
	}
	wg.Wait()
}

func (h *Handler) handleGetAlbum(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if core.IsRemoteAlbumID(id) && h.OnDemand != nil {
		h.respondRemoteAlbum(w, r, id)
		return
	}
	album, err := h.Catalog.GetAlbum(r.Context(), id)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "Album not found")
		return
	}
	user := userFrom(r.Context())
	trackAnn, _ := h.Annotations.AnnotationMap(r.Context(), user.ID, models.ItemTrack)
	albumAnn, _ := h.Annotations.Get(r.Context(), user.ID, models.ItemAlbum, id)

	songs := h.albumSongs(r, album, trackAnn)
	// Reflect the enriched (local + remote) tracklist in the album totals.
	album.SongCount = len(songs)
	album.Duration = 0
	for _, s := range songs {
		album.Duration += s.Duration
	}

	resp := newResponse()
	out := toAlbumID3(album, &albumAnn, songs)
	resp.Album = &out
	write(w, r, resp)
}

// albumTrackKey is the dedup key matching a remote track against an owned one
// (by normalized title).
func albumTrackKey(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
}

// albumSongs returns an album's songs: the local tracks plus — when a content
// provider is configured — the rest of the album's tracks fetched from the
// provider (the ones the user does not own, as remote play-on-demand entries),
// deduped by title and ordered by disc/track so the album reads in order.
func (h *Handler) albumSongs(r *http.Request, album models.Album, trackAnn map[string]models.Annotation) []Child {
	ctx := r.Context()
	local, _ := h.Catalog.ListTracksByAlbum(ctx, album.ID)
	songs := make([]Child, 0, len(local))
	seen := make(map[string]bool, len(local))
	for _, t := range local {
		songs = append(songs, toChild(t, annPtr(trackAnn, t.ID)))
		if k := albumTrackKey(t.Title); k != "" {
			seen[k] = true
		}
	}

	if h.OnDemand != nil && strings.TrimSpace(album.Name) != "" {
		rctx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		if remote, err := h.OnDemand.RemoteTracksForAlbum(rctx, album.ArtistName, album.Name); err == nil {
			for _, t := range remote {
				k := albumTrackKey(t.Title)
				if k == "" || seen[k] {
					continue
				}
				seen[k] = true
				t.AlbumID = album.ID // keep the client on this album page
				songs = append(songs, toChild(t, h.localAnn(rctx, trackAnn, t.ID)))
			}
		}
	}

	sort.SliceStable(songs, func(i, j int) bool {
		if songs[i].DiscNumber != songs[j].DiscNumber {
			return songs[i].DiscNumber < songs[j].DiscNumber
		}
		return songs[i].Track < songs[j].Track
	})
	return songs
}

func (h *Handler) handleGetAlbumList2(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	opt := buildAlbumListOptions(r, user.ID)
	albums, err := h.Catalog.ListAlbums(r.Context(), opt)
	if err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	albumAnn, _ := h.Annotations.AnnotationMap(r.Context(), user.ID, models.ItemAlbum)
	list := make([]AlbumID3, 0, len(albums))
	for _, a := range albums {
		list = append(list, toAlbumID3(a, annPtr(albumAnn, a.ID), nil))
	}
	resp := newResponse()
	resp.AlbumList2 = &AlbumList2{Album: list}
	write(w, r, resp)
}

func (h *Handler) handleGetSong(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	t, err := h.Catalog.GetTrack(r.Context(), id)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "Song not found")
		return
	}
	user := userFrom(r.Context())
	ann, _ := h.Annotations.Get(r.Context(), user.ID, models.ItemTrack, id)
	resp := newResponse()
	child := toChild(t, &ann)
	resp.Song = &child
	write(w, r, resp)
}

func (h *Handler) handleGetGenres(w http.ResponseWriter, r *http.Request) {
	genres, err := h.Genres.List(r.Context())
	if err != nil {
		writeError(w, r, ErrGeneric, err.Error())
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
	ctx := r.Context()
	resp := newResponse()
	out := &Starred2{}

	artistIDs, _ := h.Annotations.ListStarred(ctx, user.ID, models.ItemArtist)
	for _, id := range artistIDs {
		if a, err := h.Catalog.GetArtist(ctx, id); err == nil {
			out.Artist = append(out.Artist, toArtistID3(a, nil, nil))
		}
	}
	albumIDs, _ := h.Annotations.ListStarred(ctx, user.ID, models.ItemAlbum)
	for _, id := range albumIDs {
		if a, err := h.Catalog.GetAlbum(ctx, id); err == nil {
			out.Album = append(out.Album, toAlbumID3(a, nil, nil))
		}
	}
	songIDs, _ := h.Annotations.ListStarred(ctx, user.ID, models.ItemTrack)
	for _, id := range songIDs {
		if t, err := h.Catalog.GetTrack(ctx, id); err == nil {
			out.Song = append(out.Song, toChild(t, nil))
		}
	}
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
	letters := make([]string, 0, len(buckets))
	for k := range buckets {
		letters = append(letters, k)
	}
	sort.Strings(letters)
	out := make([]Index, 0, len(letters))
	for _, l := range letters {
		out = append(out, Index{Name: l, Artist: buckets[l]})
	}
	return out
}

func sortedIndexID3(buckets map[string][]ArtistID3) []IndexID3 {
	letters := make([]string, 0, len(buckets))
	for k := range buckets {
		letters = append(letters, k)
	}
	sort.Strings(letters)
	out := make([]IndexID3, 0, len(letters))
	for _, l := range letters {
		out = append(out, IndexID3{Name: l, Artist: buckets[l]})
	}
	return out
}
