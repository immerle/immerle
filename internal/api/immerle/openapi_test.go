package immerle

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	chi "github.com/go-chi/chi/v5"
)

// TestEveryRouteIsInOpenAPISpec fails when a mounted /api/v1 route has no
// @Router annotation, so the generated TS client never silently misses one.
func TestEveryRouteIsInOpenAPISpec(t *testing.T) {
	raw, err := os.ReadFile("../docs/swagger.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}

	routes := chi.NewRouter()
	NewHandler(Deps{}).Register(routes)
	err = chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		path, ok := strings.CutPrefix(route, "/api/v1")
		if !ok {
			return nil // legacy /rest alias, outside the documented API
		}
		if _, ok := spec.Paths[path][strings.ToLower(method)]; !ok {
			t.Errorf("%s %s is mounted but missing from the OpenAPI spec (add a @Router annotation)", method, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
