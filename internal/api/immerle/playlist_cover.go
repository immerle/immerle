package immerle

import (
	"encoding/json"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"net/http"
	"os"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/covergen"
	"github.com/immerle/immerle/internal/models"
)

// handlePlaylistCover replaces a playlist's cover with an uploaded image
// (multipart field "file"). Owner-only. Mirrors handleTrackCover; bypasses the
// typed client like all cover uploads.
func (h *Handler) handlePlaylistCover(w http.ResponseWriter, r *http.Request) {
	p, ok := h.coverTarget(w, r)
	if !ok {
		return
	}
	data, ok := readCoverUpload(w, r)
	if !ok {
		return
	}
	h.storePlaylistCover(w, r, p, data)
}

// handlePlaylistCoverGenerate renders a cover from a JSON spec (multipart field
// "spec", covergen.Spec) plus an optional background image (field "file").
// Owner-only.
func (h *Handler) handlePlaylistCoverGenerate(w http.ResponseWriter, r *http.Request) {
	p, ok := h.coverTarget(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCoverBytes)
	if err := r.ParseMultipartForm(maxCoverBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "malformed multipart form")
		return
	}
	var spec covergen.Spec
	if err := json.Unmarshal([]byte(r.FormValue("spec")), &spec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid cover spec")
		return
	}

	var bg image.Image
	if file, _, err := r.FormFile("file"); err == nil {
		defer func() { _ = file.Close() }()
		if img, _, err := image.Decode(file); err == nil {
			bg = img
		}
	}

	data, err := covergen.Render(r.Context(), spec, bg)
	if err != nil {
		writeInternal(w, err)
		return
	}
	h.storePlaylistCover(w, r, p, data)
}

// coverTarget loads the playlist and enforces owner-only cover edits.
func (h *Handler) coverTarget(w http.ResponseWriter, r *http.Request) (models.Playlist, bool) {
	p, err := h.playlistSvc.CoverTarget(r.Context(), userFrom(r.Context()), pathParam(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return models.Playlist{}, false
	}
	return p, true
}

// storePlaylistCover writes the cover bytes under a fresh id, points the
// playlist at it, removes the previous custom cover, and returns the playlist.
func (h *Handler) storePlaylistCover(w http.ResponseWriter, r *http.Request, p models.Playlist, data []byte) {
	coverID := uuid.NewString()
	if err := os.MkdirAll(h.CoversDir, 0o755); err != nil {
		writeInternal(w, err)
		return
	}
	if err := os.WriteFile(coverPath(h.CoversDir, coverID), data, 0o644); err != nil {
		writeInternal(w, err)
		return
	}
	if err := h.playlistSvc.SaveCover(r.Context(), p.ID, coverID); err != nil {
		_ = os.Remove(coverPath(h.CoversDir, coverID))
		writeInternal(w, err)
		return
	}
	if p.CoverArt != "" {
		_ = os.Remove(coverPath(h.CoversDir, p.CoverArt))
	}
	d, err := h.playlistSvc.Get(r.Context(), userFrom(r.Context()), p.ID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, detailToView(d))
}
