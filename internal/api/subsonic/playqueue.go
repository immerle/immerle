package subsonic

import (
	"net/http"
)

func (h *Handler) handleGetPlayQueue(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	res, err := h.playQueueSvc.Get(r.Context(), user.ID)
	if err != nil {
		if isNotFound(err) {
			writeOK(w, r) // no saved queue
			return
		}
		h.failInternal(w, r, err)
		return
	}
	q := res.Queue
	resp := newResponse()
	resp.PlayQueue = &PlayQueue{
		Current:   q.Current,
		Position:  q.PositionMs,
		Username:  user.Username,
		Changed:   formatTime(q.ChangedAt),
		ChangedBy: q.ChangedBy,
		Entry:     trackEntriesToChildren(res.Entries),
	}
	write(w, r, resp)
}

func (h *Handler) handleSavePlayQueue(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	// The Subsonic API has no concept of a playing/paused flag, or of rich
	// per-track metadata, on this endpoint (see the spec's savePlayQueue) —
	// a Subsonic client's queue save can't drive the native app's
	// cross-device remote-control feature, and any not-yet-downloaded
	// remote track it references won't survive being mirrored elsewhere.
	err := h.playQueueSvc.Save(r.Context(), user.ID, param(r, "current"),
		int64(intParam(r, "position", 0)), false, param(r, "c"), r.Form["id"], nil)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}
