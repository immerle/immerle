// Package httputil holds small request helpers shared across the API handlers.
package httputil

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the best-effort client IP (first X-Forwarded-For hop, else
// the remote address without its port).
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// APITokenFromRequest extracts a personal API token / device JWT from the
// Authorization Bearer header or the apiKey parameter. r.ParseForm must have
// been called.
func APITokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return r.Form.Get("apiKey")
}
