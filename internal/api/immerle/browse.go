package immerle

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// intQuery reads a query parameter as an int, returning def when absent or
// malformed.
func intQuery(r *http.Request, name string, def int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// This file exposes the catalog browse resources (artists, albums) over the
// shared core.LibraryService — the same business logic the Subsonic handler
// uses, rendered as native REST resources.

// artistView is the REST representation of an artist. Albums is populated only
// on the single-artist resource.
type artistView struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	AlbumCount int         `json:"albumCount"`
	CoverArt   string      `json:"coverArt,omitempty"`
	Starred    *time.Time  `json:"starred,omitempty"`
	Albums     []albumView `json:"albums,omitempty"`
}

// albumView is the REST representation of an album. Tracks is populated on the
// single-album resource (and on an artist's albums when songs are requested).
type albumView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Artist    string     `json:"artist"`
	ArtistID  string     `json:"artistId"`
	CoverArt  string     `json:"coverArt,omitempty"`
	SongCount int        `json:"songCount"`
	Duration  int        `json:"duration"`
	Year      int        `json:"year,omitempty"`
	Genre     string     `json:"genre,omitempty"`
	Starred   *time.Time `json:"starred,omitempty"`
	Tracks    []songView `json:"tracks,omitempty"`
}

func toArtistView(a models.Artist, ann *models.Annotation, albums []albumView) artistView {
	v := artistView{ID: a.ID, Name: a.Name, AlbumCount: a.AlbumCount, CoverArt: a.CoverArt, Albums: albums}
	if ann != nil {
		v.Starred = ann.Starred
	}
	return v
}

func toAlbumView(a models.Album, ann *models.Annotation, tracks []songView) albumView {
	v := albumView{
		ID: a.ID, Name: a.Name, Artist: a.ArtistName, ArtistID: a.ArtistID,
		CoverArt: a.CoverArt, SongCount: a.SongCount, Duration: a.Duration,
		Year: a.Year, Genre: a.Genre, Tracks: tracks,
	}
	if ann != nil {
		v.Starred = ann.Starred
	}
	return v
}

func albumEntriesToView(entries []core.AlbumEntry) []albumView {
	out := make([]albumView, 0, len(entries))
	for _, e := range entries {
		out = append(out, toAlbumView(e.Album, e.Annotation, trackEntriesToSongViews(e.Tracks)))
	}
	return out
}

func trackEntriesToSongViews(entries []core.TrackEntry) []songView {
	if len(entries) == 0 {
		return nil
	}
	out := make([]songView, 0, len(entries))
	for _, e := range entries {
		out = append(out, toSongView(e.Track))
	}
	return out
}

// annPtr returns a pointer to the annotation for id, or nil when absent.
func annPtr(m map[string]models.Annotation, id string) *models.Annotation {
	if a, ok := m[id]; ok {
		return &a
	}
	return nil
}

// writeServiceError maps an application-layer error from core to the native
// error envelope and an HTTP status; unexpected errors become a 500.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, persistence.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, core.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "not authorized")
	case errors.Is(err, core.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
	default:
		writeInternal(w, err)
	}
}

// handleListArtists lists every artist with the caller's starred state.
//
// @Summary      List artists
// @Description  Returns every artist in the catalog with the caller's per-artist starred state.
// @Tags         catalog
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string][]artistView
// @Failure      401  {object}  errorResponse
// @Router       /artists [get]
func (h *Handler) handleListArtists(w http.ResponseWriter, r *http.Request) {
	artists, starred, err := h.library.Artists(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]artistView, 0, len(artists))
	for _, a := range artists {
		out = append(out, toArtistView(a, annPtr(starred, a.ID), nil))
	}
	writeResource(w, http.StatusOK, map[string]any{"artists": out})
}

// handleGetArtist returns one artist with its albums.
//
// @Summary      Get artist
// @Description  Returns an artist with its (local + remote) albums. With ?songs=true each album's tracks are inlined.
// @Tags         catalog
// @Security     BearerAuth
// @Produce      json
// @Param        id    path   string  true   "Artist id"
// @Param        songs query  bool    false  "Inline each album's tracks"
// @Success      200  {object}  artistView
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /artists/{id} [get]
func (h *Handler) handleGetArtist(w http.ResponseWriter, r *http.Request) {
	res, err := h.library.GetArtist(r.Context(), userFrom(r.Context()).ID, pathParam(r, "id"), r.URL.Query().Get("songs") == "true")
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, toArtistView(res.Artist, res.Annotation, albumEntriesToView(res.Albums)))
}

// handleGetAlbum returns one album with its merged tracklist.
//
// @Summary      Get album
// @Description  Returns an album with its merged (local + remote) tracklist.
// @Tags         catalog
// @Security     BearerAuth
// @Produce      json
// @Param        id   path   string  true  "Album id"
// @Success      200  {object}  albumView
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /albums/{id} [get]
func (h *Handler) handleGetAlbum(w http.ResponseWriter, r *http.Request) {
	res, err := h.library.GetAlbum(r.Context(), userFrom(r.Context()).ID, pathParam(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, toAlbumView(res.Album, res.Annotation, trackEntriesToSongViews(res.Tracks)))
}

// handleGetSong returns a single track.
//
// @Summary      Get song
// @Description  Returns a single track by id.
// @Tags         catalog
// @Security     BearerAuth
// @Produce      json
// @Param        id   path   string  true  "Track id"
// @Success      200  {object}  songView
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /songs/{id} [get]
func (h *Handler) handleGetSong(w http.ResponseWriter, r *http.Request) {
	te, err := h.library.Song(r.Context(), userFrom(r.Context()).ID, pathParam(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, toSongView(te.Track))
}

// handleListAlbums returns a list of albums by the requested criteria.
//
// @Summary      List albums
// @Description  Returns albums filtered/sorted by type (newest, recent, frequent, random, alphabeticalByName, byGenre, byYear, starred) with paging.
// @Tags         catalog
// @Security     BearerAuth
// @Produce      json
// @Param        type      query  string  false  "List type"  default(alphabeticalByName)
// @Param        size      query  int     false  "Page size"  default(10)
// @Param        offset    query  int     false  "Offset"
// @Param        genre     query  string  false  "Genre (for byGenre)"
// @Param        fromYear  query  int     false  "From year (for byYear)"
// @Param        toYear    query  int     false  "To year (for byYear)"
// @Success      200  {object}  map[string][]albumView
// @Failure      401  {object}  errorResponse
// @Router       /albums [get]
func (h *Handler) handleListAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := h.library.AlbumList(r.Context(), persistence.AlbumListOptions{
		Type:     r.URL.Query().Get("type"),
		Size:     intQuery(r, "size", 10),
		Offset:   intQuery(r, "offset", 0),
		Genre:    r.URL.Query().Get("genre"),
		FromYear: intQuery(r, "fromYear", 0),
		ToYear:   intQuery(r, "toYear", 0),
		UserID:   userFrom(r.Context()).ID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, map[string]any{"albums": albumEntriesToView(albums)})
}

// genreView is the REST representation of a genre with its catalog counts.
type genreView struct {
	Name       string `json:"name"`
	SongCount  int    `json:"songCount"`
	AlbumCount int    `json:"albumCount"`
}

// handleGetGenres returns the catalog's genres with their counts.
//
// @Summary      List genres
// @Description  Returns every genre with its song and album counts.
// @Tags         catalog
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string][]genreView
// @Failure      401  {object}  errorResponse
// @Router       /genres [get]
func (h *Handler) handleGetGenres(w http.ResponseWriter, r *http.Request) {
	genres, err := h.Genres.List(r.Context())
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]genreView, 0, len(genres))
	for _, g := range genres {
		out = append(out, genreView{Name: g.Name, SongCount: g.SongCount, AlbumCount: g.AlbumCount})
	}
	writeResource(w, http.StatusOK, map[string]any{"genres": out})
}

// searchView is the result of a catalog search.
type searchView struct {
	Artists []artistView `json:"artists"`
	Albums  []albumView  `json:"albums"`
	Songs   []songView   `json:"songs"`
}

// handleSearch searches the catalog (merging remote-provider results).
//
// @Summary      Search the catalog
// @Description  Searches artists, albums and songs (merging remote-provider results when enabled).
// @Tags         catalog
// @Security     BearerAuth
// @Produce      json
// @Param        q            query  string  true   "Search query"
// @Param        artistCount  query  int     false  "Max artists"  default(20)
// @Param        albumCount   query  int     false  "Max albums"   default(20)
// @Param        songCount    query  int     false  "Max songs"    default(20)
// @Success      200  {object}  searchView
// @Failure      401  {object}  errorResponse
// @Router       /search [get]
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	res, err := h.library.Search(r.Context(), userFrom(r.Context()).ID, r.URL.Query().Get("q"),
		intQuery(r, "artistCount", 20), intQuery(r, "albumCount", 20), intQuery(r, "songCount", 20))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := searchView{
		Artists: make([]artistView, 0, len(res.Artists)),
		Albums:  make([]albumView, 0, len(res.Albums)),
		Songs:   make([]songView, 0, len(res.Tracks)),
	}
	for _, a := range res.Artists {
		out.Artists = append(out.Artists, toArtistView(a, nil, nil))
	}
	for _, a := range res.Albums {
		out.Albums = append(out.Albums, toAlbumView(a, annPtr(res.AlbumAnnotations, a.ID), nil))
	}
	for _, t := range res.Tracks {
		out.Songs = append(out.Songs, toSongView(t))
	}
	writeResource(w, http.StatusOK, out)
}
