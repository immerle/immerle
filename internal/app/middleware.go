package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/persistence"
)

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
func shareHandler(shares *persistence.ShareRepo, logger *slog.Logger) http.HandlerFunc {
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
