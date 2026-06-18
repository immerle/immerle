// Package webui embeds the exported web app (expo export --platform web ->
// ui/dist) and serves it from the Go binary.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// all: is required — the export puts hashed assets under _expo/, and plain
// //go:embed skips paths whose name starts with "_" or ".".
//
//go:embed all:dist
var dist embed.FS

// Handler serves the embedded web app. Static files are served as-is; unknown
// paths fall back to index.html for client-side (expo-router) navigation.
// When no real build is embedded (placeholder only), it returns 404 so it never
// shadows the API.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	return handler(sub)
}

func handler(sub fs.FS) http.Handler {
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return http.NotFoundHandler() // not built in
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(sub, p); err == nil {
				files.ServeHTTP(w, r)
				return
			}
		}
		// ponytail: serve the SPA shell for HTML navigations; everything else
		// (missing asset, stray API path) stays a 404 instead of returning HTML.
		if p == "" || (r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html")) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(index)
			return
		}
		http.NotFound(w, r)
	})
}
