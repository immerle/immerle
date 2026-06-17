package subsonic

import (
	"net/http"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/stream"
)

func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request) {
	h.serveAudio(w, r, stream.Options{
		MaxBitRate: intParam(r, "maxBitRate", 0),
		Format:     param(r, "format"),
	})
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	// download serves the original bytes (no transcoding).
	h.serveAudio(w, r, stream.Options{Format: "raw"})
}

// serveAudio serves a track for playback/download. For a remote (provider) track
// that is not yet local, the first listen is streamed *progressively*: bytes are
// teed from the provider to the client and to disk at once, so playback starts
// immediately instead of waiting for the whole download. The saved copy is then
// ingested in the background, and later plays go through the normal (transcoding,
// seekable) local path.
func (h *Handler) serveAudio(w http.ResponseWriter, r *http.Request, opts stream.Options) {
	id := param(r, "id")
	user := userFrom(r.Context())

	if core.IsRemoteID(id) && h.OnDemand != nil {
		track, local, pending, err := h.OnDemand.PrepareStream(r.Context(), user.ID, id)
		if err != nil {
			writeError(w, r, ErrDataNotFound, "Song not found")
			return
		}
		if !local {
			h.streamProgressive(w, r, pending, opts)
			return
		}
		h.serveLocal(w, r, track, opts)
		return
	}

	track, err := h.Catalog.GetTrack(r.Context(), id)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "Song not found")
		return
	}
	h.serveLocal(w, r, track, opts)
}

func (h *Handler) serveLocal(w http.ResponseWriter, r *http.Request, track models.Track, opts stream.Options) {
	user := userFrom(r.Context())
	if h.NowPlaying != nil {
		h.NowPlaying.Set(user.ID, user.Username, track.ID)
	}
	if err := h.Streamer.Serve(w, r, track, opts); err != nil {
		h.Logger.Warn("stream failed", "track", track.ID, "error", err)
	}
}

// streamProgressive serves the provider's original bytes directly on a first
// listen. It does NOT transcode (that would force buffering the whole file
// first), so it must advertise the content type of the *actual* bytes — the
// provider's suffix — not the requested transcode format, or a client would get
// e.g. raw FLAC labelled audio/mpeg. Seeking is disabled until the local copy is
// ready; later plays go through the seekable, transcoding local path.
func (h *Handler) streamProgressive(w http.ResponseWriter, r *http.Request, pending *core.PendingDownload, opts stream.Options) {
	w.Header().Set("Content-Type", audioContentType("", pending.Suffix()))
	w.Header().Set("Accept-Ranges", "none")
	if err := h.OnDemand.StreamPending(r.Context(), pending, w); err != nil {
		// Headers/bytes have already started; nothing to do but log.
		h.Logger.Warn("progressive stream failed", "error", err)
	}
}

// audioContentType returns the MIME type to advertise. The requested format wins
// (so the client sees the transcode it asked for), falling back to the provider's
// actual suffix when no real format was requested.
func audioContentType(format, suffix string) string {
	f := strings.ToLower(format)
	if f == "" || f == "raw" {
		f = strings.ToLower(suffix)
	}
	switch f {
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

// resolveTrackForPlayback fetches a local track, or for a remote (provider) id
// triggers the on-demand download and returns the resulting local track.
func (h *Handler) resolveTrackForPlayback(r *http.Request, id string) (models.Track, error) {
	if core.IsRemoteID(id) && h.OnDemand != nil {
		user := userFrom(r.Context())
		track, _, _, err := h.OnDemand.Resolve(r.Context(), user.ID, id)
		return track, err
	}
	return h.Catalog.GetTrack(r.Context(), id)
}

func (h *Handler) handleScrobble(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	ids := r.Form["id"]
	if len(ids) == 0 {
		writeError(w, r, ErrMissingParameter, "Required parameter id is missing")
		return
	}
	submission := boolParam(r, "submission", true)

	for _, id := range ids {
		// Resolve remote ids to local so stats attach to a real track.
		track, err := h.resolveTrackForPlayback(r, id)
		if err != nil {
			continue
		}
		if h.NowPlaying != nil {
			h.NowPlaying.Set(user.ID, user.Username, track.ID)
		}
		if !submission {
			continue
		}
		if user.ScrobbleEnabled {
			at := time.Now()
			if t := intParam(r, "time", 0); t > 0 {
				at = time.UnixMilli(int64(t))
			}
			_ = h.Scrobbles.Insert(r.Context(), models.Scrobble{
				ID:        newID(),
				UserID:    user.ID,
				TrackID:   track.ID,
				PlayedAt:  at,
				Submitted: true,
			})
			_ = h.Annotations.IncrementPlay(r.Context(), user.ID, models.ItemTrack, track.ID, at)
			_ = h.Annotations.IncrementPlay(r.Context(), user.ID, models.ItemAlbum, track.AlbumID, at)
		}
		if h.Activity != nil {
			_ = h.Activity.Record(r.Context(), user, "listen", models.ItemTrack, track.ID)
		}
	}
	writeOK(w, r)
}

func (h *Handler) handleGetNowPlaying(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	out := &NowPlaying{}
	if h.NowPlaying != nil {
		for _, e := range h.NowPlaying.List() {
			track, err := h.Catalog.GetTrack(r.Context(), e.TrackID)
			if err != nil {
				continue
			}
			entry := NowPlayingEntry{
				Child:      toChild(track, nil),
				Username:   e.Username,
				MinutesAgo: int(time.Since(e.At).Minutes()),
			}
			out.Entry = append(out.Entry, entry)
		}
	}
	resp.NowPlaying = out
	write(w, r, resp)
}
