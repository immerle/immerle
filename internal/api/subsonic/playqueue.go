package subsonic

import (
	"net/http"
	"time"

	"github.com/immerle/immerle/internal/models"
)

func (h *Handler) handleGetPlayQueue(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	q, err := h.PlayQueues.Get(r.Context(), user.ID)
	if err != nil {
		if isNotFound(err) {
			writeOK(w, r) // no saved queue
			return
		}
		h.failInternal(w, r, err)
		return
	}
	trackAnn, _ := h.Annotations.AnnotationMap(r.Context(), user.ID, models.ItemTrack)
	entries := make([]Child, 0, len(q.TrackIDs))
	for _, id := range q.TrackIDs {
		t, err := h.Catalog.GetTrack(r.Context(), id)
		if err != nil {
			continue
		}
		entries = append(entries, toChild(t, annPtr(trackAnn, id)))
	}
	resp := newResponse()
	resp.PlayQueue = &PlayQueue{
		Current:   q.Current,
		Position:  q.PositionMs,
		Username:  user.Username,
		Changed:   formatTime(q.ChangedAt),
		ChangedBy: q.ChangedBy,
		Entry:     entries,
	}
	write(w, r, resp)
}

func (h *Handler) handleSavePlayQueue(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	ids := r.Form["id"]
	q := models.PlayQueue{
		UserID:     user.ID,
		Current:    param(r, "current"),
		PositionMs: int64(intParam(r, "position", 0)),
		ChangedBy:  param(r, "c"),
		ChangedAt:  time.Now(),
		TrackIDs:   ids,
	}
	if err := h.PlayQueues.Save(r.Context(), q); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}
