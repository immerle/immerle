package subsonic

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/immerle/immerle/internal/models"
)

// trimQuery normalizes a Subsonic search query (clients may quote it; "" means all).
func trimQuery(q string) string {
	return strings.Trim(strings.TrimSpace(q), "\"")
}

// toAlbumChild renders an album as a directory-style Child (used by file-based
// browsing and the v1 list/search endpoints).
func toAlbumChild(a models.Album, ann *models.Annotation) Child {
	c := Child{
		ID:       a.ID,
		Parent:   a.ArtistID,
		IsDir:    true,
		Title:    a.Name,
		Album:    a.Name,
		Artist:   a.ArtistName,
		CoverArt: coverIDForAlbum(a),
		Year:     a.Year,
		Genre:    a.Genre,
		Created:  formatTime(a.CreatedAt),
		AlbumID:  a.ID,
		ArtistID: a.ArtistID,
		Duration: a.Duration,
	}
	if ann != nil {
		c.Starred = starredStr(ann)
		c.UserRating = ann.Rating
	}
	return c
}

func (h *Handler) handleGetSongsByGenre(w http.ResponseWriter, r *http.Request) {
	genre := param(r, "genre")
	if genre == "" {
		writeError(w, r, ErrMissingParameter, "Required parameter genre is missing")
		return
	}
	count := intParam(r, "count", 10)
	offset := intParam(r, "offset", 0)
	tracks, err := h.Catalog.ListTracksByGenre(r.Context(), genre, count, offset)
	if err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	resp := newResponse()
	resp.SongsByGenre = &Songs{Song: h.tracksToChildren(r, tracks)}
	write(w, r, resp)
}

func (h *Handler) handleGetRandomSongs(w http.ResponseWriter, r *http.Request) {
	size := intParam(r, "size", 10)
	tracks, err := h.Catalog.RandomTracks(r.Context(), size, param(r, "genre"), intParam(r, "fromYear", 0), intParam(r, "toYear", 0))
	if err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	resp := newResponse()
	resp.RandomSongs = &Songs{Song: h.tracksToChildren(r, tracks)}
	write(w, r, resp)
}

func (h *Handler) handleGetAlbumList(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	opt := buildAlbumListOptions(r, user.ID)
	albums, err := h.Catalog.ListAlbums(r.Context(), opt)
	if err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	albumAnn, _ := h.Annotations.AnnotationMap(r.Context(), user.ID, models.ItemAlbum)
	list := make([]Child, 0, len(albums))
	for _, a := range albums {
		list = append(list, toAlbumChild(a, annPtr(albumAnn, a.ID)))
	}
	resp := newResponse()
	resp.AlbumList = &AlbumList{Album: list}
	write(w, r, resp)
}

func (h *Handler) handleGetMusicDirectory(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if id == "" {
		writeError(w, r, ErrMissingParameter, "Required parameter id is missing")
		return
	}
	// Remote (provider) artist/album directories.
	if h.OnDemand != nil && h.remoteMusicDirectory(w, r, id) {
		return
	}
	user := userFrom(r.Context())
	ctx := r.Context()

	// Artist directory → albums as sub-directories.
	if artist, err := h.Catalog.GetArtist(ctx, id); err == nil {
		albums, _ := h.Catalog.ListAlbumsByArtist(ctx, id)
		albumAnn, _ := h.Annotations.AnnotationMap(ctx, user.ID, models.ItemAlbum)
		children := make([]Child, 0, len(albums))
		for _, a := range albums {
			children = append(children, toAlbumChild(a, annPtr(albumAnn, a.ID)))
		}
		resp := newResponse()
		resp.Directory = &Directory{ID: artist.ID, Name: artist.Name, Child: children}
		write(w, r, resp)
		return
	}

	// Album directory → songs (local + provider enrichment, same as getAlbum).
	if album, err := h.Catalog.GetAlbum(ctx, id); err == nil {
		user := userFrom(ctx)
		trackAnn, _ := h.Annotations.AnnotationMap(ctx, user.ID, models.ItemTrack)
		resp := newResponse()
		resp.Directory = &Directory{
			ID:     album.ID,
			Name:   album.Name,
			Parent: album.ArtistID,
			Child:  h.albumSongs(r, album, trackAnn),
		}
		write(w, r, resp)
		return
	}

	writeError(w, r, ErrDataNotFound, "Directory not found")
}

func (h *Handler) handleGetStarred(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	ctx := r.Context()
	resp := newResponse()
	out := &Starred{}

	artistIDs, _ := h.Annotations.ListStarred(ctx, user.ID, models.ItemArtist)
	for _, id := range artistIDs {
		if a, err := h.Catalog.GetArtist(ctx, id); err == nil {
			out.Artist = append(out.Artist, ArtistItem{ID: a.ID, Name: a.Name})
		}
	}
	albumIDs, _ := h.Annotations.ListStarred(ctx, user.ID, models.ItemAlbum)
	for _, id := range albumIDs {
		if a, err := h.Catalog.GetAlbum(ctx, id); err == nil {
			out.Album = append(out.Album, toAlbumChild(a, nil))
		}
	}
	songIDs, _ := h.Annotations.ListStarred(ctx, user.ID, models.ItemTrack)
	for _, id := range songIDs {
		if t, err := h.Catalog.GetTrack(ctx, id); err == nil {
			out.Song = append(out.Song, toChild(t, nil))
		}
	}
	resp.Starred = out
	write(w, r, resp)
}

func (h *Handler) handleSearch2(w http.ResponseWriter, r *http.Request) {
	query := trimQuery(param(r, "query"))
	artists, albums, tracks, err := h.Catalog.Search(r.Context(), query,
		intParam(r, "artistCount", 20), intParam(r, "albumCount", 20), intParam(r, "songCount", 20))
	if err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	out := &SearchResult2{}
	seenArtist := map[string]bool{}
	for _, a := range artists {
		out.Artist = append(out.Artist, ArtistItem{ID: a.ID, Name: a.Name})
		seenArtist[strings.ToLower(a.Name)] = true
	}
	if h.OnDemand != nil && trimQuery(param(r, "query")) != "" {
		if remote, err := h.OnDemand.RemoteSearchArtists(r.Context(), trimQuery(param(r, "query")), intParam(r, "artistCount", 20)); err == nil {
			for _, a := range remote {
				if seenArtist[strings.ToLower(a.Name)] {
					continue
				}
				seenArtist[strings.ToLower(a.Name)] = true
				out.Artist = append(out.Artist, ArtistItem{ID: a.ID, Name: a.Name})
			}
		}
	}
	for _, a := range albums {
		out.Album = append(out.Album, toAlbumChild(a, nil))
	}
	out.Song = h.mergeRemoteSongs(r, h.tracksToChildren(r, tracks), trimQuery(param(r, "query")), intParam(r, "songCount", 20))
	resp := newResponse()
	resp.SearchResult2 = out
	write(w, r, resp)
}

func (h *Handler) handleGetTopSongs(w http.ResponseWriter, r *http.Request) {
	artistName := param(r, "artist")
	count := intParam(r, "count", 50)
	user := userFrom(r.Context())
	resp := newResponse()
	out := &Songs{}
	if artist, err := h.Catalog.FindArtistByName(r.Context(), artistName); err == nil {
		// Pull the artist's catalog, then rank by the user's play count.
		tracks, _ := h.Catalog.ListTracksByArtist(r.Context(), artist.ID, 1000)
		ann, _ := h.Annotations.AnnotationMap(r.Context(), user.ID, models.ItemTrack)
		sort.SliceStable(tracks, func(i, j int) bool {
			return ann[tracks[i].ID].PlayCount > ann[tracks[j].ID].PlayCount
		})
		if len(tracks) > count {
			tracks = tracks[:count]
		}
		for _, t := range tracks {
			out.Song = append(out.Song, toChild(t, annPtr(ann, t.ID)))
		}
	}
	resp.TopSongs = out
	write(w, r, resp)
}

func (h *Handler) handleGetSimilarSongs(w http.ResponseWriter, r *http.Request) {
	songs := h.similarSongs(r)
	resp := newResponse()
	resp.SimilarSongs = &SimilarSongs{Song: songs}
	write(w, r, resp)
}

func (h *Handler) handleGetSimilarSongs2(w http.ResponseWriter, r *http.Request) {
	songs := h.similarSongs(r)
	resp := newResponse()
	resp.SimilarSongs2 = &SimilarSongs2{Song: songs}
	write(w, r, resp)
}

// similarSongs is a lightweight heuristic: songs sharing the seed item's genre
// (falling back to random) since no external recommendation source is wired.
func (h *Handler) similarSongs(r *http.Request) []Child {
	count := intParam(r, "count", 50)
	genre := ""
	if t, err := h.Catalog.GetTrack(r.Context(), param(r, "id")); err == nil {
		genre = t.Genre
	}
	tracks, err := h.Catalog.RandomTracks(r.Context(), count, genre, 0, 0)
	if err != nil {
		return nil
	}
	return h.tracksToChildren(r, tracks)
}

func (h *Handler) handleGetArtistInfo(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	out := &ArtistInfo{}
	if a, err := h.Catalog.GetArtist(r.Context(), param(r, "id")); err == nil {
		out.ArtistInfoBase = h.artistInfoBase(r, a)
	}
	resp.ArtistInfo = out
	write(w, r, resp)
}

func (h *Handler) handleGetArtistInfo2(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	out := &ArtistInfo2{}
	if a, err := h.Catalog.GetArtist(r.Context(), param(r, "id")); err == nil {
		out.ArtistInfoBase = h.artistInfoBase(r, a)
	}
	resp.ArtistInfo2 = out
	write(w, r, resp)
}

// artistInfoBase fills MBID and avatar image URLs (when an avatar was fetched by
// the enrichment service) for getArtistInfo/2.
func (h *Handler) artistInfoBase(r *http.Request, a models.Artist) ArtistInfoBase {
	base := ArtistInfoBase{MusicBrainzID: a.MBID}
	if a.CoverArt != "" {
		base.SmallImageURL = h.coverURL(r, a.CoverArt, 160)
		base.MediumImageURL = h.coverURL(r, a.CoverArt, 320)
		base.LargeImageURL = h.coverURL(r, a.CoverArt, 0)
	}
	return base
}

// coverURL builds an absolute getCoverArt URL, echoing the caller's auth params
// so the returned link is directly usable.
func (h *Handler) coverURL(r *http.Request, coverArt string, size int) string {
	if coverArt == "" {
		return ""
	}
	q := url.Values{}
	for _, k := range []string{"u", "t", "s", "p", "c", "v"} {
		if v := r.Form.Get(k); v != "" {
			q.Set(k, v)
		}
	}
	q.Set("id", coverArt)
	if size > 0 {
		q.Set("size", strconv.Itoa(size))
	}
	return strings.TrimRight(h.BaseURL, "/") + "/rest/getCoverArt?" + q.Encode()
}

func (h *Handler) handleGetAlbumInfo(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	out := &AlbumInfo{}
	if a, err := h.Catalog.GetAlbum(r.Context(), param(r, "id")); err == nil {
		out.MusicBrainzID = a.MBID
	}
	resp.AlbumInfo = out
	write(w, r, resp)
}

func (h *Handler) handleGetLyrics(w http.ResponseWriter, r *http.Request) {
	// No lyrics source is wired; return an empty (valid) lyrics element.
	resp := newResponse()
	resp.Lyrics = &Lyrics{Artist: param(r, "artist"), Title: param(r, "title")}
	write(w, r, resp)
}

func (h *Handler) handleGetLyricsBySongID(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	resp.LyricsList = &LyricsList{}
	write(w, r, resp)
}

// ---- empty-but-valid stubs for sections we do not implement ----

func (h *Handler) handleGetVideos(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	resp.Videos = &Videos{}
	write(w, r, resp)
}

func (h *Handler) handleGetBookmarks(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	resp.Bookmarks = &Bookmarks{}
	write(w, r, resp)
}

func (h *Handler) handleGetInternetRadioStations(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	resp.InternetRadioStations = &InternetRadioStations{}
	write(w, r, resp)
}

func (h *Handler) handleGetChatMessages(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	resp.ChatMessages = &ChatMessages{}
	write(w, r, resp)
}

// tracksToChildren converts tracks to Child entries enriched with the caller's
// annotations.
func (h *Handler) tracksToChildren(r *http.Request, tracks []models.Track) []Child {
	user := userFrom(r.Context())
	ann, _ := h.Annotations.AnnotationMap(r.Context(), user.ID, models.ItemTrack)
	out := make([]Child, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, toChild(t, annPtr(ann, t.ID)))
	}
	return out
}
