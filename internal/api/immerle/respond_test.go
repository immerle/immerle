package immerle

import (
	"net/http"
	"net/http/httptest"
	"testing"

	chi "github.com/go-chi/chi/v5"
)

// chi returns the raw (percent-encoded) path segment; pathParam must unescape it
// so remote ids like "rart:..." aren't seen as "rart%3A..." and lose their prefix.
func TestPathParamUnescapesColon(t *testing.T) {
	var got string
	r := chi.NewRouter()
	r.Get("/artists/{id}", func(w http.ResponseWriter, req *http.Request) {
		got = pathParam(req, "id")
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/artists/rart%3AZGVl")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if want := "rart:ZGVl"; got != want {
		t.Fatalf("pathParam = %q, want %q", got, want)
	}
}
