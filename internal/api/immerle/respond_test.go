package immerle

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/persistence"
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

// A failed lookup is a 404 only when the item does not exist: a database
// failure must surface as a 500, not as "not found".
func TestWriteLookupError(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{persistence.ErrNotFound, http.StatusNotFound},
		{fmt.Errorf("load: %w", persistence.ErrNotFound), http.StatusNotFound},
		{errors.New("database is locked"), http.StatusInternalServerError},
	} {
		rec := httptest.NewRecorder()
		writeLookupError(rec, tc.err, "session not found")
		if rec.Code != tc.want {
			t.Errorf("%v: status %d, want %d", tc.err, rec.Code, tc.want)
		}
	}
}
