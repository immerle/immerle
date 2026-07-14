// Package logging configures structured JSON logging with slog and fans it
// out to live subscribers (the admin log-stream SSE endpoint).
package logging

import (
	"bytes"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
)

// historySize is how many recent log lines a freshly-connected stream
// subscriber gets immediately, so it isn't blank until the next log line.
const historySize = 200

// Hub is an io.Writer that broadcasts every log line it receives (one per
// slog record) to subscribed readers, in addition to writing it to stdout.
type Hub struct {
	mu     sync.Mutex
	subs   map[chan []byte]struct{}
	recent [][]byte
}

func newHub() *Hub {
	return &Hub{subs: make(map[chan []byte]struct{})}
}

// Write implements io.Writer. slog's JSON handler calls this once per
// complete record (already newline-terminated), serialized by the handler's
// own internal lock, so no record is ever split across calls.
func (h *Hub) Write(p []byte) (int, error) {
	line := slices.Clone(bytes.TrimRight(p, "\n"))

	h.mu.Lock()
	h.recent = append(h.recent, line)
	if len(h.recent) > historySize {
		h.recent = h.recent[len(h.recent)-historySize:]
	}
	for ch := range h.subs {
		select {
		case ch <- line:
		default: // slow subscriber: drop rather than block logging
		}
	}
	h.mu.Unlock()

	return os.Stdout.Write(p)
}

// Subscribe registers a new listener, returning it, a snapshot of recent
// history (oldest first), and an unsubscribe func to release it.
func (h *Hub) Subscribe() (ch chan []byte, history [][]byte, unsubscribe func()) {
	ch = make(chan []byte, 64)

	h.mu.Lock()
	history = slices.Clone(h.recent)
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	return ch, history, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
		close(ch)
	}
}

// New builds a *slog.Logger that writes JSON lines (level "debug"|"info"|
// "warn"|"error") and a Hub that mirrors every line for live streaming.
func New(level string) (*slog.Logger, *Hub) {
	hub := newHub()
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	return slog.New(slog.NewJSONHandler(hub, opts)), hub
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
