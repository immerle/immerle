package immerle

import (
	"errors"
	"net/http"
	"time"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

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
