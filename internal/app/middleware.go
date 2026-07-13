package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/persistence"
)

// recoverMiddleware is the outermost middleware: it recovers from a panic in any
// downstream handler, logs the stack, and writes a generic 500 so one bad request
// can't drop the connection (net/http's default) or take down the process.
func recoverMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// http.ErrAbortHandler is the sanctioned way to abort a response; the
			// server handles it specially, so re-panic instead of swallowing it.
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			logger.Error("panic recovered",
				"method", r.Method,
				"path", r.URL.Path,
				"panic", rec,
				"stack", string(debug.Stack()),
			)
			// Best effort: if the handler already wrote headers (e.g. mid-stream)
			// this is a no-op, but recovering still prevents the crash.
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal server error"}`))
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the response status for access logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// securityHeadersMiddleware sets baseline security response headers on every
// response (cheap, applies globally including to error and preflight responses).
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs each request at debug level.
func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"elapsed", time.Since(start),
		)
	})
}

// shareHandler serves the public landing for a share secret, incrementing its
// view count. It returns share metadata as JSON (clients build the playback UI).
func shareHandler(shares *persistence.ShareRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret := strings.TrimPrefix(r.URL.Path, "/share/")
		if secret == "" {
			http.NotFound(w, r)
			return
		}
		share, err := shares.GetBySecret(r.Context(), secret)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if share.ExpiresAt != nil && share.ExpiresAt.Before(time.Now()) {
			http.Error(w, "Share expired", http.StatusGone)
			return
		}
		_ = shares.IncrementViews(r.Context(), share.ID)

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          share.ID,
			"itemType":    share.ItemType,
			"itemId":      share.ItemID,
			"description": share.Description,
			"createdAt":   share.CreatedAt,
		})
	}
}
