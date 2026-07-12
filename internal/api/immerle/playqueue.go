package immerle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/persistence"
)

// This file exposes the saved play queue (cross-device sync) and the now-playing
// feed over the shared core.PlayQueueService and the now-playing tracker.

// playQueueView is the caller's saved play queue resolved into its tracks.
type playQueueView struct {
	Current  string `json:"current,omitempty"`
	Position int64  `json:"position"`
	// Playing reports whether Current was playing (vs paused) as of
	// ChangedAt — see playQueueRequest.Playing.
	Playing   bool       `json:"playing"`
	ChangedBy string     `json:"changedBy,omitempty"`
	ChangedAt *time.Time `json:"changedAt,omitempty"`
	Entries   []songView `json:"entries"`
	// TargetDeviceID, when set, is the id of the sole device that should be
	// actively playing this queue right now — every other device should
	// pause instead of doubling the audio. Empty means unrestricted: each
	// device manages its own playback independently (the default).
	TargetDeviceID string `json:"targetDeviceId,omitempty"`
}

func toPlayQueueView(res core.PlayQueueResult) playQueueView {
	v := playQueueView{
		Current:        res.Queue.Current,
		Position:       res.Queue.PositionMs,
		Playing:        res.Queue.Playing,
		ChangedBy:      res.Queue.ChangedBy,
		Entries:        make([]songView, 0, len(res.Entries)),
		TargetDeviceID: res.Queue.TargetDeviceID,
	}
	if !res.Queue.ChangedAt.IsZero() {
		v.ChangedAt = &res.Queue.ChangedAt
	}
	for _, e := range res.Entries {
		v.Entries = append(v.Entries, toSongViewAnnotated(e.Track, e.Annotation))
	}
	return v
}

// handleGetPlayQueue returns the caller's saved play queue (an empty queue when
// none has been saved).
//
// @Summary  Get play queue
// @Description  Returns the caller's saved cross-device play queue with its tracks.
// @Tags     playback
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  playQueueView
// @Failure  401  {object}  errorResponse
// @Router   /play-queue [get]
func (h *Handler) handleGetPlayQueue(w http.ResponseWriter, r *http.Request) {
	res, err := h.playQueue.Get(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			writeResource(w, http.StatusOK, playQueueView{Entries: []songView{}})
			return
		}
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, toPlayQueueView(res))
}

// handleStreamPlayQueue streams the caller's play-queue state over
// Server-Sent Events — the real-time channel behind cross-device sync and
// remote control (see ui/src/audio/store.ts, web only: native falls back to
// polling since React Native has no EventSource).
//
// @Summary      Stream play-queue events (SSE)
// @Description  Server-Sent Events stream. Emits the current queue immediately, then again on every change (save, target change).
// @Tags         playback
// @Security     BearerAuth
// @Produce      text/event-stream
// @Success      200  {string}  string  "SSE stream"
// @Failure      401  {object}  errorResponse
// @Router       /play-queue/events [get]
func (h *Handler) handleStreamPlayQueue(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeInternal(w, fmt.Errorf("streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsubscribe := h.playQueue.Subscribe(user.ID)
	defer unsubscribe()

	// Bound each write so a stalled/slow client connection errors out instead
	// of leaking this goroutine and its subscription forever.
	rc := http.NewResponseController(w)
	setDeadline := func() { _ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second)) }

	// Send the current snapshot immediately so a freshly-opened client is in
	// sync without waiting on someone else's next change.
	res, err := h.playQueue.Get(r.Context(), user.ID)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		writeInternal(w, err)
		return
	}
	setDeadline()
	writePlayQueueEvent(w, flusher, toPlayQueueView(res))

	// Keep-alive so idle connections (and dead peers behind a proxy) are detected.
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			setDeadline()
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			setDeadline()
			writePlayQueueEvent(w, flusher, toPlayQueueView(ev.Queue))
		}
	}
}

func writePlayQueueEvent(w http.ResponseWriter, flusher http.Flusher, view playQueueView) {
	payload, _ := json.Marshal(view)
	fmt.Fprintf(w, "event: state\ndata: %s\n\n", payload)
	flusher.Flush()
}

// playQueueRequest is the body for PUT /play-queue.
type playQueueRequest struct {
	IDs      []string `json:"ids"`
	Current  string   `json:"current"`
	Position int64    `json:"position"`
	// Playing reports whether Current is playing (vs paused). A spectator
	// device (see TargetDeviceID) can also use this write to push a
	// play/pause/skip command — the active device applies it once it
	// notices Current/Playing changed on its next poll.
	Playing bool   `json:"playing"`
	Client  string `json:"client"`
}

// handleSavePlayQueue persists the caller's play queue.
//
// @Summary  Save play queue
// @Description  Replaces the caller's saved play queue (tracks, current track, position and playing state).
// @Tags     playback
// @Security BearerAuth
// @Accept   json
// @Param    body  body  playQueueRequest  true  "Play queue"
// @Success  204  "No Content"
// @Failure  400  {object}  errorResponse
// @Failure  401  {object}  errorResponse
// @Router   /play-queue [put]
func (h *Handler) handleSavePlayQueue(w http.ResponseWriter, r *http.Request) {
	var req playQueueRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.playQueue.Save(r.Context(), userFrom(r.Context()).ID, req.Current, req.Position, req.Playing, req.Client, req.IDs); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// playbackTargetView is a device the caller can transfer active playback to.
type playbackTargetView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

// handleListPlaybackTargets lists the caller's recently-active app installs —
// the devices playback can be transferred to.
//
// @Summary  List playback targets
// @Description  Lists the caller's recently-active app installs (device-kind API tokens), for the "cast to device" picker.
// @Tags     playback
// @Security BearerAuth
// @Produce  json
// @Success  200  {array}  playbackTargetView
// @Failure  401  {object}  errorResponse
// @Router   /play-queue/targets [get]
func (h *Handler) handleListPlaybackTargets(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.Auth.ListDeviceSessions(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]playbackTargetView, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, playbackTargetView{ID: s.ID, Name: s.Name, LastUsedAt: s.LastUsedAt})
	}
	writeResource(w, http.StatusOK, out)
}

// setPlaybackTargetRequest is the body for PUT /play-queue/target.
type setPlaybackTargetRequest struct {
	// DeviceID is the device to make the sole active player; empty clears the
	// restriction so every device plays independently again.
	DeviceID string `json:"deviceId"`
}

// handleSetPlaybackTarget assigns or clears the caller's active playback
// device.
//
// @Summary  Set the active playback device
// @Description  Assigns (or, with an empty deviceId, clears) the sole device that should be actively playing the caller's queue.
// @Tags     playback
// @Security BearerAuth
// @Accept   json
// @Param    body  body  setPlaybackTargetRequest  true  "Target device"
// @Success  204  "No Content"
// @Failure  400  {object}  errorResponse
// @Failure  401  {object}  errorResponse
// @Router   /play-queue/target [put]
func (h *Handler) handleSetPlaybackTarget(w http.ResponseWriter, r *http.Request) {
	var req setPlaybackTargetRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.playQueue.SetTarget(r.Context(), userFrom(r.Context()).ID, req.DeviceID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// nowPlayingView is one entry in the now-playing feed.
type nowPlayingView struct {
	Song       songView `json:"song"`
	Username   string   `json:"username"`
	MinutesAgo int      `json:"minutesAgo"`
}

// handleNowPlaying returns what every user is currently playing.
//
// @Summary  Now playing
// @Description  Returns the tracks every user is currently playing, newest first.
// @Tags     playback
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  map[string][]nowPlayingView
// @Failure  401  {object}  errorResponse
// @Router   /now-playing [get]
func (h *Handler) handleNowPlaying(w http.ResponseWriter, r *http.Request) {
	out := make([]nowPlayingView, 0)
	if h.NowPlaying != nil {
		for _, e := range h.NowPlaying.List() {
			track, err := h.Catalog.GetTrack(r.Context(), e.TrackID)
			if err != nil {
				continue
			}
			out = append(out, nowPlayingView{
				Song:       toSongView(track),
				Username:   e.Username,
				MinutesAgo: int(time.Since(e.At).Minutes()),
			})
		}
	}
	writeResource(w, http.StatusOK, map[string]any{"nowPlaying": out})
}
