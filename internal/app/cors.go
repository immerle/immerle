package app

import (
	"net/http"
	"strconv"
	"strings"
)

// corsMiddleware applies CORS headers for the allowed origins (read live via
// origins() so the admin can change them at runtime) and answers preflight
// (OPTIONS) requests. An entry of "*" allows any origin. Because immerle
// authenticates via query parameters (not cookies), credentials are not
// required; a matched specific origin is echoed back so browser clients on that
// origin can read responses.
func corsMiddleware(origins func() []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowAny := false
		set := make(map[string]bool)
		for _, o := range origins() {
			o = strings.TrimSpace(o)
			if o == "*" {
				allowAny = true
				continue
			}
			if o != "" {
				set[strings.ToLower(o)] = true
			}
		}

		origin := r.Header.Get("Origin")
		if origin != "" && (allowAny || set[strings.ToLower(origin)]) {
			h := w.Header()
			// Vary so caches don't serve one origin's response to another.
			h.Add("Vary", "Origin")
			if allowAny {
				h.Set("Access-Control-Allow-Origin", "*")
			} else {
				h.Set("Access-Control-Allow-Origin", origin)
			}
			h.Set("Access-Control-Allow-Methods", "GET, POST, HEAD, OPTIONS")
			// Echo requested headers, falling back to a sensible default set.
			reqHeaders := r.Header.Get("Access-Control-Request-Headers")
			if reqHeaders == "" {
				reqHeaders = "Authorization, Content-Type, Range"
			}
			h.Set("Access-Control-Allow-Headers", reqHeaders)
			// Expose range/streaming headers so players can seek.
			h.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
			h.Set("Access-Control-Max-Age", strconv.Itoa(86400))
		}

		// Short-circuit preflight requests.
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
