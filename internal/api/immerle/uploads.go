package immerle

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/scanner"
)

// maxUploadBytes caps a single uploaded audio file (200 MiB — lossless album
// tracks fit comfortably). ponytail: a flat per-request cap, raise it here if
// users need bigger files.
const maxUploadBytes = 200 << 20

// maxCoverBytes caps an uploaded cover image (10 MiB).
const maxCoverBytes = 10 << 20

// songView is the Subsonic-compatible song shape the web UI's TrackList expects.
// It mirrors the Subsonic `Child`, with the cover-art id falling back to the
// album id (as the Subsonic serializer does).
type songView struct {
	ID              string            `json:"id"`
	Title           string            `json:"title"`
	Album           string            `json:"album"`
	Artist          string            `json:"artist"`
	AlbumID         string            `json:"albumId"`
	ArtistID        string            `json:"artistId"`
	CoverArt        string            `json:"coverArt"`
	Duration        int               `json:"duration"`
	Track           int               `json:"track,omitempty"`
	Year            int               `json:"year,omitempty"`
	Composer        string            `json:"composer,omitempty"`
	Genre           string            `json:"genre,omitempty"`
	BPM             int               `json:"bpm,omitempty"`
	ReplayGainTrack float64           `json:"replayGainTrack,omitempty"`
	ReplayGainAlbum float64           `json:"replayGainAlbum,omitempty"`
	TitleSort       string            `json:"titleSort,omitempty"`
	Work            string            `json:"work,omitempty"`
	MovementName    string            `json:"movementName,omitempty"`
	MovementNo      int               `json:"movementNumber,omitempty"`
	Lyrics          string            `json:"lyrics,omitempty"`
	Participants    []participantView `json:"participants,omitempty"`
	Suffix          string            `json:"suffix,omitempty"`
	ContentType     string            `json:"contentType,omitempty"`
	Size            int64             `json:"size,omitempty"`
	// Per-user annotation state, populated only on catalog reads (album/artist/
	// song/search/playlist). Absent on upload/admin views, where it is not loaded.
	Starred   *time.Time `json:"starred,omitempty"`
	Rating    int        `json:"rating,omitempty"`
	PlayCount int        `json:"playCount,omitempty"`
	// Unresolved marks a federated-playlist entry not yet matched to a local
	// track: id is empty and only title/artist/album are populated. Resolve it
	// via POST /playlists/{id}/tracks/{position}/resolve before playing.
	Unresolved bool `json:"unresolved,omitempty"`
	// Remote marks a track not yet downloaded (an on-demand provider result,
	// not a row in the local catalog). Playing it streams progressively — the
	// server can't yet serve byte ranges for it, so seeking isn't available
	// until the background download finishes and it's replayed.
	Remote bool `json:"remote,omitempty"`
}

// participantView mirrors models.Participant locally so the OpenAPI generator
// (swag) can resolve it without cross-package parsing. Same JSON shape.
type participantView struct {
	Role string `json:"role"`
	Name string `json:"name"`
}

func toSongView(t models.Track) songView {
	cover := t.CoverArt
	if cover == "" {
		cover = t.AlbumID
	}
	var participants []participantView
	for _, p := range t.Participants {
		participants = append(participants, participantView{Role: p.Role, Name: p.Name})
	}
	return songView{
		ID: t.ID, Title: t.Title, Album: t.AlbumName, Artist: t.ArtistName,
		AlbumID: t.AlbumID, ArtistID: t.ArtistID, CoverArt: cover, Duration: t.Duration,
		Track: t.TrackNo, Year: t.Year, Composer: t.Composer, Genre: t.Genre, Suffix: t.Suffix,
		ContentType: t.ContentType, Size: t.Size,
		BPM: t.BPM, ReplayGainTrack: t.ReplayGainTrack, ReplayGainAlbum: t.ReplayGainAlbum,
		TitleSort: t.TitleSort, Work: t.Work, MovementName: t.MovementName, MovementNo: t.MovementNo,
		Lyrics: t.Lyrics, Participants: participants, Unresolved: t.Unresolved, Remote: t.Remote,
	}
}

// handleLocalSongs lists the tracks the caller uploaded, newest first.
func (h *Handler) handleLocalSongs(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	tracks, err := h.Catalog.ListUploadedBy(r.Context(), user.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	songs := make([]songView, 0, len(tracks))
	for _, t := range tracks {
		songs = append(songs, toSongView(t))
	}
	writeResource(w, http.StatusOK, map[string]any{"songs": songs})
}

// handleUpload ingests a single uploaded audio file into the catalog and marks
// the caller as its owner.
func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if h.Scanner == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "uploads are not configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "missing audio file (multipart field \"file\")")
		return
	}
	defer func() { _ = file.Close() }()

	name := safeFilename(header.Filename)
	if _, ok := scanner.IsAudioFile(name); !ok {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "unsupported audio format")
		return
	}

	dir := filepath.Join(h.UploadsDir, user.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeInternal(w, err)
		return
	}
	dest := uniquePath(dir, name)
	if err := saveFile(dest, file); err != nil {
		writeInternal(w, err)
		return
	}

	trackID, err := h.Scanner.IngestFile(r.Context(), dest)
	if err != nil {
		_ = os.Remove(dest)
		writeError(w, http.StatusBadRequest, "ingest_failed", "could not read the audio file")
		return
	}
	if err := h.Catalog.SetTrackOwner(r.Context(), trackID, user.ID); err != nil {
		writeInternal(w, err)
		return
	}
	t, err := h.Catalog.GetTrack(r.Context(), trackID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusCreated, toSongView(t))
}

// trackTitleRequest renames a track.
type trackTitleRequest struct {
	Title *string `json:"title"`
}

// handleTrackUpdate renames a track the caller uploaded.
func (h *Handler) handleTrackUpdate(w http.ResponseWriter, r *http.Request) {
	t, ok := h.ownedTrack(w, r)
	if !ok {
		return
	}
	var req trackTitleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "validation", "title must not be empty")
			return
		}
		if err := h.Catalog.SetTrackTitle(r.Context(), t.ID, title); err != nil {
			writeInternal(w, err)
			return
		}
	}
	updated, err := h.Catalog.GetTrack(r.Context(), t.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, toSongView(updated))
}

// handleTrackCover replaces the cover art of a track the caller uploaded.
func (h *Handler) handleTrackCover(w http.ResponseWriter, r *http.Request) {
	t, ok := h.ownedTrack(w, r)
	if !ok {
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
	writeResource(w, http.StatusOK, toSongView(updated))
}

// coverPath is the on-disk path of a custom cover file.
func coverPath(coversDir, coverID string) string {
	return filepath.Join(coversDir, coverID)
}

// readCoverUpload reads and validates the "file" multipart field as an image.
// On any failure it writes the response and returns ok=false.
func readCoverUpload(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCoverBytes)
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "missing image file (multipart field \"file\")")
		return nil, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		writeInternal(w, err)
		return nil, false
	}
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "cover must be an image")
		return nil, false
	}
	return data, true
}

// saveCustomCover writes image data as a fresh cover file, points the track at it
// and removes the track's previous custom cover (never a shared album cover). A
// fresh id each time sidesteps the cover cache (keyed by id), so the new image
// shows immediately. Returns the refreshed track.
func (h *Handler) saveCustomCover(ctx context.Context, t models.Track, data []byte) (models.Track, error) {
	coverID := uuid.NewString()
	if err := os.MkdirAll(h.CoversDir, 0o755); err != nil {
		return models.Track{}, err
	}
	if err := os.WriteFile(coverPath(h.CoversDir, coverID), data, 0o644); err != nil {
		return models.Track{}, err
	}
	if t.CoverArt != "" && t.CoverArt != t.AlbumID {
		_ = os.Remove(coverPath(h.CoversDir, t.CoverArt))
	}
	if err := h.Catalog.SetTrackCover(ctx, t.ID, coverID); err != nil {
		return models.Track{}, err
	}
	return h.Catalog.GetTrack(ctx, t.ID)
}

// handleTrackDelete removes a track the caller uploaded: its catalog row, the
// audio file on disk (so a rescan won't re-ingest it), and any custom cover.
func (h *Handler) handleTrackDelete(w http.ResponseWriter, r *http.Request) {
	t, ok := h.ownedTrack(w, r)
	if !ok {
		return
	}
	if err := h.Catalog.DeleteTrack(r.Context(), t.ID); err != nil {
		writeInternal(w, err)
		return
	}
	if t.Path != "" {
		_ = os.Remove(t.Path)
	}
	if t.CoverArt != "" && t.CoverArt != t.AlbumID {
		_ = os.Remove(filepath.Join(h.CoversDir, t.CoverArt))
	}
	writeResource(w, http.StatusNoContent, nil)
}

// ownedTrack loads the {id} track and asserts the caller uploaded it. On any
// failure it writes the response and returns ok=false.
func (h *Handler) ownedTrack(w http.ResponseWriter, r *http.Request) (models.Track, bool) {
	user := userFrom(r.Context())
	t, err := h.Catalog.GetTrack(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "track not found")
		return models.Track{}, false
	}
	if t.UploadedBy == "" || t.UploadedBy != user.ID {
		writeError(w, http.StatusForbidden, "forbidden", "you can only edit tracks you uploaded")
		return models.Track{}, false
	}
	return t, true
}

// safeFilename strips any directory components from an uploaded filename.
func safeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		return "upload"
	}
	return name
}

// uniquePath returns dir/name, prefixing a short unique token if it already
// exists, so same-named uploads never clobber each other.
func uniquePath(dir, name string) string {
	dest := filepath.Join(dir, name)
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}
	return filepath.Join(dir, uuid.NewString()[:8]+"-"+name)
}

func saveFile(dest string, src io.Reader) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, src); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}
