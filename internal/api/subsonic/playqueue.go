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
	// Subsonic's savePlayQueue lacks a playing/paused flag, shuffle/repeat mode, and
	// rich per-track metadata, so a Subsonic client's save can't drive cross-device
	// remote control, doesn't preserve not-yet-downloaded remote tracks when mirrored,
	// and resets shuffle/repeat to their defaults (accepted interop degradation, same
	// as the other two).
	err := h.playQueueSvc.Save(r.Context(), user.ID, param(r, "current"),
		int64(intParam(r, "position", 0)), false, param(r, "c"), r.Form["id"], nil, false, "")
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}
