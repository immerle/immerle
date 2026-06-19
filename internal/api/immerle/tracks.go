package immerle

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/immerle/immerle/internal/persistence"
)

// This file holds the admin library-management endpoints (browse/edit/delete any
// track). It reuses the song shape (toSongView) and cover helpers (saveCustomCover,
// readCoverUpload) shared with the user-facing "local" uploads in uploads.go.

// handleAdminTracks lists downloaded tracks (paginated, searchable).
//
// @Summary      List library tracks
// @Description  Admin only. Lists downloaded (local) tracks, newest first, with optional search and pagination.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Param        query   query  string  false  "Case-insensitive search over title/artist/album"
// @Param        limit   query  int     false  "Page size (default 50, max 200)"
// @Param        offset  query  int     false  "Offset for pagination"
// @Success      200  {object}  TrackListDTO
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /admin/tracks [get]
func (h *Handler) handleAdminTracks(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	limit := atoiDefault(r.URL.Query().Get("limit"), 50)
	if limit > 200 {
		limit = 200
	}
	offset := atoiDefault(r.URL.Query().Get("offset"), 0)

	tracks, err := h.Catalog.ListAllTracks(r.Context(), persistence.TrackListOptions{Query: query, Size: limit, Offset: offset})
	if err != nil {
		writeInternal(w, err)
		return
	}
	total, err := h.Catalog.CountTracks(r.Context(), query)
	if err != nil {
		writeInternal(w, err)
		return
	}
	songs := make([]songView, 0, len(tracks))
	for _, t := range tracks {
		songs = append(songs, toSongView(t))
	}
	writeResource(w, http.StatusOK, map[string]any{
		"tracks": songs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// trackUpdateRequest is the body for PATCH /admin/tracks/{id}. All fields are
// optional; omitted fields keep their current value.
type trackUpdateRequest struct {
	Title   *string `json:"title"`
	Genre   *string `json:"genre"`
	Year    *int    `json:"year"`
	TrackNo *int    `json:"trackNo"`
	DiscNo  *int    `json:"discNo"`
}

// handleAdminTrackUpdate edits a track's simple metadata.
//
// @Summary      Edit track metadata
// @Description  Admin only. Edits a track's title, genre, year and track/disc number. Album and artist links are not changed.
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  string              true  "Track id"
// @Param        body  body  trackUpdateRequest  true  "Fields to update (all optional)"
// @Success      200  {object}  TrackDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /admin/tracks/{id} [patch]
func (h *Handler) handleAdminTrackUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := pathParam(r, "id")
	var req trackUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	t, err := h.Catalog.GetTrack(r.Context(), id)
	if errors.Is(err, persistence.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "track not found")
		return
	}
	if err != nil {
		writeInternal(w, err)
		return
	}
	// Merge provided fields over the current values.
	title, genre, year, trackNo, discNo := t.Title, t.Genre, t.Year, t.TrackNo, t.DiscNo
	if req.Title != nil {
		title = strings.TrimSpace(*req.Title)
	}
	if req.Genre != nil {
		genre = strings.TrimSpace(*req.Genre)
	}
	if req.Year != nil {
		year = *req.Year
	}
	if req.TrackNo != nil {
		trackNo = *req.TrackNo
	}
	if req.DiscNo != nil {
		discNo = *req.DiscNo
	}
	if title == "" {
		writeError(w, http.StatusBadRequest, "validation", "title is required")
		return
	}
	if err := h.Catalog.UpdateTrackMetadata(r.Context(), id, title, genre, year, trackNo, discNo); err != nil {
		writeInternal(w, err)
		return
	}
	updated, err := h.Catalog.GetTrack(r.Context(), id)
	if err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("track metadata edited", "track", id, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, toSongView(updated))
}

// handleAdminTrackCover replaces any track's cover art from an uploaded image.
//
// @Summary      Upload track cover
// @Description  Admin only. Replaces a single track's cover art with an uploaded image (multipart form field "file").
// @Tags         admin
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        id    path  string  true  "Track id"
// @Param        file  formData  file  true  "Cover image (jpeg/png/gif/webp)"
// @Success      200  {object}  TrackDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      415  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /admin/tracks/{id}/cover [put]
func (h *Handler) handleAdminTrackCover(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	t, err := h.Catalog.GetTrack(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "track not found")
		return
	}
	data, ok := readCoverUpload(w, r)
	if !ok {
		return
	}
	updated, err := h.saveCustomCover(r.Context(), t, data)
	if err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("track cover replaced", "track", t.ID, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, toSongView(updated))
}

// handleAdminTrackDelete deletes a track: the audio file, the DB row (and the
// rows referencing it), and any custom cover.
//
// @Summary      Delete a track
// @Description  Admin only. Removes the audio file and the track and all rows referencing it (annotations, shares, activity, downloads, playlist entries, scrobbles).
// @Tags         admin
// @Security     BearerAuth
// @Param        id  path  string  true  "Track id"
// @Success      204  "No Content"
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /admin/tracks/{id} [delete]
func (h *Handler) handleAdminTrackDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := pathParam(r, "id")
	t, err := h.Catalog.GetTrack(r.Context(), id)
	if errors.Is(err, persistence.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "track not found")
		return
	}
	if err != nil {
		writeInternal(w, err)
		return
	}
	// Cascade clears rows that have no ON DELETE CASCADE FK to tracks
	// (annotations, shares, activity_events, download_jobs).
	if err := h.Catalog.DeleteTrackCascade(r.Context(), id); err != nil {
		writeInternal(w, err)
		return
	}
	if t.Path != "" {
		_ = os.Remove(t.Path)
	}
	if t.CoverArt != "" && t.CoverArt != t.AlbumID {
		_ = os.Remove(coverPath(h.CoversDir, t.CoverArt))
	}
	h.Logger.Info("track deleted", "track", id, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusNoContent, nil)
}

// atoiDefault parses s as an int, returning def on empty/invalid input.
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}
