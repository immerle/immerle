package immerle

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/lyrics"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// lyricsView is a track's lyrics, plain or synced. When synced, every line
// carries a startMs (playback offset in milliseconds) so a client can highlight
// the current line (karaoke).
type lyricsView struct {
	Synced bool        `json:"synced"`
	Lines  []lyricLine `json:"lines"`
}

// lyricLine is one lyrics line; StartMs is omitted for unsynced lyrics.
type lyricLine struct {
	StartMs int64  `json:"startMs,omitempty"`
	Text    string `json:"text"`
}

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
	if n < 0 {
		// Negative sizes/offsets are meaningless for pagination; clamp to 0 so
		// they never reach the query layer (atoiDefault in tracks.go rejects them
		// the same way).
		return 0
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
		CoverArt: coverIDForAlbum(a), SongCount: a.SongCount, Duration: a.Duration,
		Year: a.Year, Genre: a.Genre, Tracks: tracks,
	}
	if ann != nil {
		v.Starred = ann.Starred
	}
	return v
}

// coverIDForAlbum falls back to the album's own id when it has no cached
// cover (its own, or any track's): the cover service resolves an album id by
// extracting embedded/sidecar art live from one of its tracks (see
// stream.CoverService.resolveOriginal), the same fallback toSongView already
// applies for a track with no cached cover — and the Subsonic API's
// coverIDForAlbum in internal/api/subsonic/convert.go already relies on.
// Without it, an album whose tracks only have embedded (never cached) art
// shows no cover at all in any album grid, while the same track plays fine
// with a visible cover once resolved via its own (equivalently-falling-back)
// song cover id.
func coverIDForAlbum(a models.Album) string {
	if a.CoverArt != "" {
		return a.CoverArt
	}
	return a.ID
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
		out = append(out, toSongViewAnnotated(e.Track, e.Annotation))
	}
	return out
}

// toSongViewAnnotated is toSongView plus the caller's per-user annotation state
// (star/rating/play count). A nil annotation leaves those fields empty.
func toSongViewAnnotated(t models.Track, ann *models.Annotation) songView {
	v := toSongView(t)
	if ann != nil {
		v.Starred = ann.Starred
		v.Rating = ann.Rating
		v.PlayCount = ann.PlayCount
	}
	return v
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
	writeResource(w, http.StatusOK, toSongViewAnnotated(te.Track, te.Annotation))
}

// songLocalStatusView reports whether a remote (on-demand) track has finished
// downloading in the background, with the resolved local song when it has.
type songLocalStatusView struct {
	Local bool      `json:"local"`
	Song  *songView `json:"song,omitempty"`
}

// handleGetSongLocalStatus reports whether a remote track id has finished
// downloading, without ever triggering a download itself — used by the player
// to upgrade a still-progressive-streaming track to the seekable local one
// once it's ready, instead of requiring the user to replay it. A non-remote
// id (already local, or unknown) just reports local=false; it's not an error.
//
// @Summary      Check whether a remote track has finished downloading
// @Description  Read-only, never downloads. Poll this while playing a remote (not-yet-local) track to know when it's safe to seek.
// @Tags         catalog
// @Security     BearerAuth
// @Produce      json
// @Param        id   path   string  true  "Track id (remote or local)"
// @Success      200  {object}  songLocalStatusView
// @Failure      401  {object}  errorResponse
// @Router       /songs/{id}/local [get]
func (h *Handler) handleGetSongLocalStatus(w http.ResponseWriter, r *http.Request) {
	if h.OnDemand != nil {
		if localID, ok := h.OnDemand.LocalTrackIDForRemote(r.Context(), pathParam(r, "id")); ok {
			if te, err := h.library.Song(r.Context(), userFrom(r.Context()).ID, localID); err == nil {
				view := toSongViewAnnotated(te.Track, te.Annotation)
				writeResource(w, http.StatusOK, songLocalStatusView{Local: true, Song: &view})
				return
			}
		}
	}
	writeResource(w, http.StatusOK, songLocalStatusView{Local: false})
}

// handleGetSongLyrics returns a track's lyrics, parsed into plain or synced lines.
//
// @Summary      Get song lyrics
// @Description  Returns a track's lyrics. When the stored tags carry [mm:ss.xx] timestamps the document is "synced" and every line has a startMs (ms).
// @Tags         catalog
// @Security     BearerAuth
// @Produce      json
// @Param        id   path   string  true  "Track id"
// @Success      200  {object}  lyricsView
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /songs/{id}/lyrics [get]
func (h *Handler) handleGetSongLyrics(w http.ResponseWriter, r *http.Request) {
	t, err := h.Catalog.GetTrack(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	doc := lyrics.Parse(t.Lyrics)
	out := lyricsView{Synced: doc.Synced, Lines: make([]lyricLine, 0, len(doc.Lines))}
	for _, l := range doc.Lines {
		out.Lines = append(out.Lines, lyricLine{StartMs: l.StartMs, Text: l.Text})
	}
	writeResource(w, http.StatusOK, out)
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

// searchHitView is one entry in a search result: exactly one of Artist/Album/
// Song/Playlist is set, per Type.
type searchHitView struct {
	Type     string        `json:"type"` // artist|album|song|playlist
	Artist   *artistView   `json:"artist,omitempty"`
	Album    *albumView    `json:"album,omitempty"`
	Song     *songView     `json:"song,omitempty"`
	Playlist *playlistView `json:"playlist,omitempty"`
}

// searchView is the result of a catalog search: every match — artists,
// albums, songs and public playlists alike — in one list, ranked by
// relevance to the query rather than grouped by type.
type searchView struct {
	Results []searchHitView `json:"results"`
}

// Songs extracts just the song hits, in ranked order — a convenience for
// callers (mainly tests) that only want the flat song list.
func (v searchView) Songs() []songView {
	var out []songView
	for _, h := range v.Results {
		if h.Song != nil {
			out = append(out, *h.Song)
		}
	}
	return out
}

// handleSearch searches the catalog and public playlists (merging
// remote-provider results), ranked together by relevance to the query.
//
// @Summary      Search the catalog
// @Description  Searches artists, albums, songs and public playlists (merging remote-provider results when enabled), returned as one list ranked by relevance to the query.
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
	q := r.URL.Query().Get("q")
	res, err := h.library.Search(r.Context(), userFrom(r.Context()).ID, q,
		intQuery(r, "artistCount", 20), intQuery(r, "albumCount", 20), intQuery(r, "songCount", 20))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	// name is kept alongside each hit purely to rank the merged list; it's not
	// part of the response (the view already carries the display name).
	type rankedHit struct {
		name string
		view searchHitView
	}
	hits := make([]rankedHit, 0, len(res.Artists)+len(res.Albums)+len(res.Tracks)+len(res.Playlists))
	for _, a := range res.Artists {
		v := toArtistView(a, nil, nil)
		hits = append(hits, rankedHit{a.Name, searchHitView{Type: "artist", Artist: &v}})
	}
	for _, a := range res.Albums {
		v := toAlbumView(a, annPtr(res.AlbumAnnotations, a.ID), nil)
		hits = append(hits, rankedHit{a.Name, searchHitView{Type: "album", Album: &v}})
	}
	for _, t := range res.Tracks {
		v := toSongViewAnnotated(t, annPtr(res.TrackAnnotations, t.ID))
		hits = append(hits, rankedHit{t.Title, searchHitView{Type: "song", Song: &v}})
	}
	for _, p := range res.Playlists {
		v := toPlaylistView(p, nil)
		hits = append(hits, rankedHit{p.Name, searchHitView{Type: "playlist", Playlist: &v}})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		return core.Relevance(q, hits[i].name) < core.Relevance(q, hits[j].name)
	})

	out := searchView{Results: make([]searchHitView, len(hits))}
	for i, hit := range hits {
		out.Results[i] = hit.view
	}
	writeResource(w, http.StatusOK, out)
}

// favoritesView is the caller's starred catalog.
type favoritesView struct {
	Artists []artistView `json:"artists"`
	Albums  []albumView  `json:"albums"`
	Songs   []songView   `json:"songs"`
}

// handleFavorites returns the items the caller has starred.
//
// @Summary  List favorites
// @Description  Returns the artists, albums and songs the caller has starred.
// @Tags     catalog
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  favoritesView
// @Failure  401  {object}  errorResponse
// @Router   /me/favorites [get]
func (h *Handler) handleFavorites(w http.ResponseWriter, r *http.Request) {
	st := h.library.Starred(r.Context(), userFrom(r.Context()).ID)
	out := favoritesView{
		Artists: make([]artistView, 0, len(st.Artists)),
		Albums:  make([]albumView, 0, len(st.Albums)),
		Songs:   make([]songView, 0, len(st.Songs)),
	}
	for _, a := range st.Artists {
		out.Artists = append(out.Artists, toArtistView(a, nil, nil))
	}
	for _, a := range st.Albums {
		out.Albums = append(out.Albums, toAlbumView(a, nil, nil))
	}
	for _, t := range st.Songs {
		out.Songs = append(out.Songs, toSongView(t))
	}
	writeResource(w, http.StatusOK, out)
}

// handleSongsByGenre returns a page of songs tagged with a genre.
//
// @Summary  List songs by genre
// @Description  Returns songs tagged with the given genre (paged).
// @Tags     catalog
// @Security BearerAuth
// @Produce  json
// @Param    genre   query  string  true   "Genre name"
// @Param    count   query  int     false  "Page size"  default(200)
// @Param    offset  query  int     false  "Offset"
// @Success  200  {object}  map[string][]songView
// @Failure  400  {object}  errorResponse
// @Failure  401  {object}  errorResponse
// @Router   /songs [get]
func (h *Handler) handleSongsByGenre(w http.ResponseWriter, r *http.Request) {
	genre := r.URL.Query().Get("genre")
	if genre == "" {
		writeValidation(w, []fieldError{{Field: "genre", Message: "genre is required"}})
		return
	}
	tracks, err := h.library.SongsByGenre(r.Context(), userFrom(r.Context()).ID, genre, intQuery(r, "count", 200), intQuery(r, "offset", 0))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, map[string]any{"songs": trackEntriesToSongViews(tracks)})
}
