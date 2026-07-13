package subsonic

import (
	"net/http"
	"time"

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

// serveAudio delegates to the shared media server, mapping a missing track to a
// Subsonic error. Mid-stream failures are handled (logged) inside the server.
func (h *Handler) serveAudio(w http.ResponseWriter, r *http.Request, opts stream.Options) {
	id := param(r, "id")
	// A downloaded podcast episode is served straight from its local file (no
	// transcoding) — its id is not a catalog track, so try it before the catalog.
	if h.Podcasts != nil {
		if ep, err := h.Podcasts.Episode(r.Context(), id); err == nil && ep.Status == "completed" && ep.MediaPath != "" {
			http.ServeFile(w, r, ep.MediaPath)
			return
		}
	}
	if err := h.media.ServeAudio(w, r, userFrom(r.Context()), id, opts); err != nil {
		writeError(w, r, ErrDataNotFound, "Song not found")
	}
}

func (h *Handler) handleScrobble(w http.ResponseWriter, r *http.Request) {
	ids := r.Form["id"]
	if len(ids) == 0 {
		writeError(w, r, ErrMissingParameter, "Required parameter id is missing")
		return
	}
	at := time.Now()
	if t := int64Param(r, "time", 0); t > 0 {
		at = time.UnixMilli(t)
	}
	h.playback.Scrobble(r.Context(), userFrom(r.Context()), ids, boolParam(r, "submission", true), at)
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
