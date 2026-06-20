package immerle

import (
	"errors"
	"net/http"
	"time"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/persistence"
)

// This file exposes the saved play queue (cross-device sync) and the now-playing
// feed over the shared core.PlayQueueService and the now-playing tracker.

// playQueueView is the caller's saved play queue resolved into its tracks.
type playQueueView struct {
	Current   string     `json:"current,omitempty"`
	Position  int64      `json:"position"`
	ChangedBy string     `json:"changedBy,omitempty"`
	ChangedAt *time.Time `json:"changedAt,omitempty"`
	Entries   []songView `json:"entries"`
}

func toPlayQueueView(res core.PlayQueueResult) playQueueView {
	v := playQueueView{
		Current:   res.Queue.Current,
		Position:  res.Queue.PositionMs,
		ChangedBy: res.Queue.ChangedBy,
		Entries:   make([]songView, 0, len(res.Entries)),
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

// playQueueRequest is the body for PUT /play-queue.
type playQueueRequest struct {
	IDs      []string `json:"ids"`
	Current  string   `json:"current"`
	Position int64    `json:"position"`
	Client   string   `json:"client"`
}

// handleSavePlayQueue persists the caller's play queue.
//
// @Summary  Save play queue
// @Description  Replaces the caller's saved play queue (tracks, current track and position).
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
	if err := h.playQueue.Save(r.Context(), userFrom(r.Context()).ID, req.Current, req.Position, req.Client, req.IDs); err != nil {
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
