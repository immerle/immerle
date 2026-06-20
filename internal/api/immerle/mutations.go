package immerle

import (
	"net/http"
	"time"
)

// This file exposes the per-user catalog mutations (favorites, ratings, plays)
// over the shared core.PlaybackService — the same logic the Subsonic star/
// setRating/scrobble endpoints use.

// setStar stars/unstars a single item of the given kind. Track stars resolve a
// remote id to its local copy and record "favorite" activity (handled by the
// service).
func (h *Handler) setStar(w http.ResponseWriter, r *http.Request, kind string, star bool) {
	id := pathParam(r, "id")
	var tracks, albums, artists []string
	switch kind {
	case "song":
		tracks = []string{id}
	case "album":
		albums = []string{id}
	case "artist":
		artists = []string{id}
	}
	h.playback.SetStarred(r.Context(), userFrom(r.Context()), tracks, albums, artists, star)
	writeResource(w, http.StatusNoContent, nil)
}

// @Summary  Favorite a song
// @Tags     favorites
// @Security BearerAuth
// @Param    id   path  string  true  "Track id"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Router   /songs/{id}/star [put]
func (h *Handler) handleStarSong(w http.ResponseWriter, r *http.Request) {
	h.setStar(w, r, "song", true)
}

// @Summary  Unfavorite a song
// @Tags     favorites
// @Security BearerAuth
// @Param    id   path  string  true  "Track id"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Router   /songs/{id}/star [delete]
func (h *Handler) handleUnstarSong(w http.ResponseWriter, r *http.Request) {
	h.setStar(w, r, "song", false)
}

// @Summary  Favorite an album
// @Tags     favorites
// @Security BearerAuth
// @Param    id   path  string  true  "Album id"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Router   /albums/{id}/star [put]
func (h *Handler) handleStarAlbum(w http.ResponseWriter, r *http.Request) {
	h.setStar(w, r, "album", true)
}

// @Summary  Unfavorite an album
// @Tags     favorites
// @Security BearerAuth
// @Param    id   path  string  true  "Album id"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Router   /albums/{id}/star [delete]
func (h *Handler) handleUnstarAlbum(w http.ResponseWriter, r *http.Request) {
	h.setStar(w, r, "album", false)
}

// @Summary  Favorite an artist
// @Tags     favorites
// @Security BearerAuth
// @Param    id   path  string  true  "Artist id"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Router   /artists/{id}/star [put]
func (h *Handler) handleStarArtist(w http.ResponseWriter, r *http.Request) {
	h.setStar(w, r, "artist", true)
}

// @Summary  Unfavorite an artist
// @Tags     favorites
// @Security BearerAuth
// @Param    id   path  string  true  "Artist id"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Router   /artists/{id}/star [delete]
func (h *Handler) handleUnstarArtist(w http.ResponseWriter, r *http.Request) {
	h.setStar(w, r, "artist", false)
}

// ratingRequest is the body for setting an item's rating (0–5).
type ratingRequest struct {
	Rating int `json:"rating"`
}

// handleSetRating rates the item at {id}. The item type (song/album/artist) is
// detected from the id, so the same handler serves all three paths.
//
// @Summary  Rate an item
// @Description  Sets the caller's rating (0–5) on the item. The item type is detected from the id.
// @Tags     ratings
// @Security BearerAuth
// @Accept   json
// @Param    id    path  string         true  "Item id (song, album or artist)"
// @Param    body  body  ratingRequest  true  "Rating"
// @Success  204  "No Content"
// @Failure  400  {object}  errorResponse
// @Failure  401  {object}  errorResponse
// @Router   /songs/{id}/rating [put]
func (h *Handler) handleSetRating(w http.ResponseWriter, r *http.Request) {
	var req ratingRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.playback.SetRating(r.Context(), userFrom(r.Context()).ID, pathParam(r, "id"), req.Rating); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// handleClearRating removes the caller's rating from the item at {id}.
//
// @Summary  Clear an item's rating
// @Tags     ratings
// @Security BearerAuth
// @Param    id   path  string  true  "Item id (song, album or artist)"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Router   /songs/{id}/rating [delete]
func (h *Handler) handleClearRating(w http.ResponseWriter, r *http.Request) {
	if err := h.playback.SetRating(r.Context(), userFrom(r.Context()).ID, pathParam(r, "id"), 0); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// scrobbleRequest is the body for POST /scrobbles. submission defaults to true;
// playedAt (epoch millis) defaults to now.
type scrobbleRequest struct {
	IDs        []string `json:"ids"`
	Submission *bool    `json:"submission"`
	PlayedAt   int64    `json:"playedAt"`
}

// handleScrobble registers playback for the given tracks: it sets the now-playing
// entry and, when submission is true and the user has scrobbling enabled, records
// a play.
//
// @Summary  Scrobble plays
// @Description  Records playback for one or more tracks (now-playing + optional submission).
// @Tags     plays
// @Security BearerAuth
// @Accept   json
// @Param    body  body  scrobbleRequest  true  "Scrobble"
// @Success  204  "No Content"
// @Failure  400  {object}  errorResponse
// @Failure  401  {object}  errorResponse
// @Router   /scrobbles [post]
func (h *Handler) handleScrobble(w http.ResponseWriter, r *http.Request) {
	var req scrobbleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.IDs) == 0 {
		writeValidation(w, []fieldError{{Field: "ids", Message: "at least one track id is required"}})
		return
	}
	submission := true
	if req.Submission != nil {
		submission = *req.Submission
	}
	at := time.Now()
	if req.PlayedAt > 0 {
		at = time.UnixMilli(req.PlayedAt)
	}
	h.playback.Scrobble(r.Context(), userFrom(r.Context()), req.IDs, submission, at)
	writeResource(w, http.StatusNoContent, nil)
}
