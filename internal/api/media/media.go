// Package media serves audio streams and cover art over HTTP. It is the shared
// presentation-level media layer used by both the Subsonic and the native REST
// API, so streaming, range handling, transcoding and the remote-provider
// progressive-download path live in exactly one place.
package media

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/stream"
)

// Server serves track audio and cover art. OnDemand and NowPlaying are optional.
type Server struct {
	catalog    *persistence.CatalogRepo
	streamer   *stream.Streamer
	cover      *stream.CoverService
	onDemand   *core.CatalogService
	nowPlaying *core.NowPlayingTracker
	logger     *slog.Logger
}

// NewServer wires the media server. onDemand and nowPlaying may be nil.
func NewServer(catalog *persistence.CatalogRepo, streamer *stream.Streamer, cover *stream.CoverService, onDemand *core.CatalogService, nowPlaying *core.NowPlayingTracker, logger *slog.Logger) *Server {
	return &Server{catalog: catalog, streamer: streamer, cover: cover, onDemand: onDemand, nowPlaying: nowPlaying, logger: logger}
}

// ServeAudio serves a track for playback/download. For a remote (provider) track
// not yet local, the first listen is streamed progressively (teed to disk).
// Returns persistence.ErrNotFound when the track does not exist (before any bytes
// are written); once it returns nil the response has been written (a mid-stream
// failure is logged, not returned).
func (s *Server) ServeAudio(w http.ResponseWriter, r *http.Request, user models.User, id string, opts stream.Options) error {
	ctx := r.Context()
	if core.IsRemoteID(id) && s.onDemand != nil {
		track, local, pending, err := s.onDemand.PrepareStream(ctx, user.ID, id)
		if err != nil {
			return persistence.ErrNotFound
		}
		if !local {
			s.streamProgressive(w, r, pending)
			return nil
		}
		s.serveLocal(w, r, user, track, opts)
		return nil
	}

	track, err := s.catalog.GetTrack(ctx, id)
	if err != nil {
		return err
	}
	s.serveLocal(w, r, user, track, opts)
	return nil
}

func (s *Server) serveLocal(w http.ResponseWriter, r *http.Request, user models.User, track models.Track, opts stream.Options) {
	if s.nowPlaying != nil {
		s.nowPlaying.Set(user.ID, user.Username, track.ID)
	}
	if err := s.streamer.Serve(w, r, track, opts); err != nil && s.logger != nil {
		s.logger.Warn("stream failed", "track", track.ID, "error", err)
	}
}

// streamProgressive serves the provider's original bytes directly on a first
// listen. It does not transcode (that would force buffering the whole file), so
// it advertises the content type of the actual bytes and disables seeking; later
// plays go through the seekable, transcoding local path.
func (s *Server) streamProgressive(w http.ResponseWriter, r *http.Request, pending *core.PendingDownload) {
	w.Header().Set("Content-Type", audioContentType(pending.Suffix()))
	w.Header().Set("Accept-Ranges", "none")
	if err := s.onDemand.StreamPending(r.Context(), pending, w); err != nil && s.logger != nil {
		s.logger.Warn("progressive stream failed", "error", err)
	}
}

// ServeCover serves cover art for an id at an optional size. Returns
// stream.ErrNoCover when no cover resolves.
func (s *Server) ServeCover(w http.ResponseWriter, r *http.Request, id string, size int) error {
	data, contentType, err := s.cover.Get(r.Context(), id, size)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	return nil
}

// audioContentType returns the MIME type to advertise for the provider's actual
// bytes, derived from its file suffix.
func audioContentType(suffix string) string {
	switch strings.ToLower(suffix) {
	case "mp3", "mpeg":
		return "audio/mpeg"
	case "flac":
		return "audio/flac"
	case "ogg", "opus", "vorbis":
		return "audio/ogg"
	case "aac", "m4a", "mp4":
		return "audio/mp4"
	case "wav":
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}
