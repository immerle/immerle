// Package docs embeds the generated OpenAPI 3.1 specification for the native
// immerle API and serves it together with a self-contained Swagger UI.
//
// The spec is generated from handler annotations with swaggo/swag v2:
//
//	make openapi   # or: swag init -g doc.go -d internal/api/immerle \
//	               #          -o internal/api/docs --ot json,yaml --parseInternal --v3.1
package docs

import (
	"embed"
	"net/http"

	chi "github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

//go:embed swagger.json swagger.yaml
var specFS embed.FS

// Register mounts the OpenAPI spec and Swagger UI:
//   - GET /openapi.json — the OpenAPI 3.1 document (JSON)
//   - GET /openapi.yaml — the same document (YAML)
//   - GET /swagger/      — Swagger UI (self-contained), reading /openapi.json
func Register(mux chi.Router) {
	mux.HandleFunc("/openapi.json", serveSpec("swagger.json", "application/json"))
	mux.HandleFunc("/openapi.yaml", serveSpec("swagger.yaml", "application/yaml"))
	mux.Handle("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/openapi.json")))
}

func serveSpec(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := specFS.ReadFile(name)
		if err != nil {
			http.Error(w, "spec not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(data)
	}
}
