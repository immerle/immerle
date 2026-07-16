package immerle

import (
	"context"
	"errors"
	"net/http"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/api/httputil"
	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/stream"
)

// mediaAuthMiddleware authorizes a stream/download request by EITHER a valid
// short-lived signed URL (exp+sig query, no user attached — the client scrobbles
// separately) OR a Bearer token (a real user, attributed to now-playing). On
// failure it answers 401.
func (h *Handler) mediaAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm() // expose exp/sig/apiKey from the query
		if h.media.VerifyToken(chi.URLParam(r, "id"), r.Form.Get("exp"), r.Form.Get("sig")) {
			next.ServeHTTP(w, r)
			return
		}
		user, err := h.Auth.Authenticate(r.Context(), core.Credentials{
			APIToken:  httputil.APITokenFromRequest(r),
			RemoteIP:  httputil.ClientIP(r),
			UserAgent: r.UserAgent(),
		})
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	})
}

// streamURLs is the signed media URLs minted for a track.
type streamURLs struct {
	Stream   string `json:"stream"`
	Download string `json:"download"`
}

// handleStreamURL mints short-lived signed stream/download URLs for the track,
// usable directly as an <audio>/<video> src (no Authorization header needed).
//
// @Summary  Mint signed media URLs
// @Description  Returns short-lived signed stream and download URLs for the track (usable as a plain media src). They expire after a few minutes.
// @Tags     media
// @Security BearerAuth
// @Produce  json
// @Param    id   path  string  true  "Track id"
// @Success  200  {object}  streamURLs
// @Failure  401  {object}  errorResponse
// @Router   /songs/{id}/stream-url [get]
func (h *Handler) handleStreamURL(w http.ResponseWriter, r *http.Request) {
	// Sign the raw (still percent-encoded) id, since that's what mediaAuthMiddleware
	// reads back via chi.URLParam — signing the decoded id breaks verification for ids needing escaping.
	id := chi.URLParam(r, "id")
	exp, sig := h.media.SignToken(id)
	q := "?exp=" + exp + "&sig=" + sig
	writeResource(w, http.StatusOK, streamURLs{
		Stream:   "/api/v1/songs/" + id + "/stream" + q,
		Download: "/api/v1/songs/" + id + "/download" + q,
	})
}

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
// The id "generator" instead renders a cover on the fly from query params,
// e.g. /cover/generator?icon=1f30d&title=charts.top50&color=%231db954&angle=45.
//
// @Summary  Cover art
// @Description  Returns the cover image for a track or album id, optionally resized. The id "generator" instead builds a cover on the fly from its own query params (icon, title, subTitle, color, color2, angle); title/subTitle may be a known i18n key (e.g. "charts.top50") resolved via locale, or literal text.
// @Tags     media
// @Security BearerAuth
// @Produce  image/jpeg
// @Param    id        path   string   true   "Track/album id, or \"generator\" for the cover builder"
// @Param    size      query  int      false  "Square size in pixels"
// @Param    locale    query  string   false  "Label language for a generator cover's title/subTitle i18n keys (e.g. \"en\", \"fr\")"
// @Param    icon      query  string   false  "Generator: Twemoji codepoint, e.g. \"1f30d\" or \"1f1eb-1f1f7\""
// @Param    title     query  string   false  "Generator: title text or i18n key"
// @Param    subTitle  query  string   false  "Generator: subtitle text or i18n key"
// @Param    color     query  string   false  "Generator: background color, hex"
// @Param    color2    query  string   false  "Generator: gradient end color, hex (empty = solid)"
// @Param    angle     query  number   false  "Generator: gradient angle, degrees"
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
