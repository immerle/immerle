package immerle

import (
	"errors"
	"net/http"

	"github.com/immerle/immerle/internal/stream"
)

// This file exposes audio streaming/download and cover art over the shared
// media server (the same code the Subsonic stream/download/getCoverArt
// endpoints use).

// handleStream streams a track's audio (transcoded per query options, seekable).
//
// @Summary  Stream a track
// @Description  Streams a track's audio. Supports HTTP range requests; maxBitRate/format transcode when set. Remote tracks are streamed progressively on first listen.
// @Tags     media
// @Security BearerAuth
// @Param    id          path   string  true   "Track id"
// @Param    maxBitRate  query  int     false  "Transcode to at most this bit rate (kbps)"
// @Param    format      query  string  false  "Transcode format (or 'raw' for original)"
// @Success  200  "Audio stream"
// @Success  206  "Partial content"
// @Failure  401  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /songs/{id}/stream [get]
func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request) {
	opts := stream.Options{MaxBitRate: intQuery(r, "maxBitRate", 0), Format: r.URL.Query().Get("format")}
	if err := h.media.ServeAudio(w, r, userFrom(r.Context()), pathParam(r, "id"), opts); err != nil {
		writeServiceError(w, err)
	}
}

// handleDownload serves a track's original bytes (no transcoding).
//
// @Summary  Download a track
// @Description  Serves a track's original audio bytes (no transcoding).
// @Tags     media
// @Security BearerAuth
// @Param    id   path  string  true  "Track id"
// @Success  200  "Audio bytes"
// @Failure  401  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /songs/{id}/download [get]
func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	if err := h.media.ServeAudio(w, r, userFrom(r.Context()), pathParam(r, "id"), stream.Options{Format: "raw"}); err != nil {
		writeServiceError(w, err)
	}
}

// handleCover serves cover art for a track or album id at an optional size.
//
// @Summary  Cover art
// @Description  Returns the cover image for a track or album id, optionally resized.
// @Tags     media
// @Security BearerAuth
// @Produce  image/jpeg
// @Param    id    path   string  true   "Track or album id"
// @Param    size  query  int     false  "Square size in pixels"
// @Success  200  "Image bytes"
// @Failure  401  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /cover/{id} [get]
func (h *Handler) handleCover(w http.ResponseWriter, r *http.Request) {
	if err := h.media.ServeCover(w, r, pathParam(r, "id"), intQuery(r, "size", 0)); err != nil {
		if errors.Is(err, stream.ErrNoCover) {
			writeError(w, http.StatusNotFound, "not_found", "cover art not found")
			return
		}
		writeInternal(w, err)
	}
}
