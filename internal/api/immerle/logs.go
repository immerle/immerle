package immerle

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// handleStreamLogs streams live server log lines (structured JSON, one per
// event) over Server-Sent Events — the feed behind the admin log viewer.
//
// @Summary      Stream server logs (SSE)
// @Description  Admin only. Server-Sent Events stream of structured JSON log lines. Sends recent history immediately, then every new line as it's logged.
// @Tags         admin
// @Security     BearerAuth
// @Produce      text/event-stream
// @Success      200  {string}  string  "SSE stream"
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Router       /admin/logs/stream [get]
func (h *Handler) handleStreamLogs(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeInternal(w, fmt.Errorf("streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, history, unsubscribe := h.LogHub.Subscribe()
	defer unsubscribe()

	h.Logger.Info("log stream connected", "user", userFrom(r.Context()).Username, "remote", r.RemoteAddr)
	defer h.Logger.Info("log stream disconnected", "user", userFrom(r.Context()).Username, "remote", r.RemoteAddr)

	rc := http.NewResponseController(w)
	setDeadline := func() { _ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second)) }

	setDeadline()
	for _, line := range history {
		if _, err := fmt.Fprintf(w, "event: log\ndata: %s\n\n", line); err != nil {
			return
		}
	}
	flusher.Flush()

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
		case line, ok := <-ch:
			if !ok {
				return
			}
			setDeadline()
			if _, err := fmt.Fprintf(w, "event: log\ndata: %s\n\n", line); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
