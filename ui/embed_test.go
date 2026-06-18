package webui

import (
	"net/http"
	"net/http/httptest"
	"testing/fstest"

	"testing"
)

func TestHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":          {Data: []byte("<html>app</html>")},
		"_expo/static/app.js": {Data: []byte("console.log(1)")},
	}
	h := handler(fsys)

	cases := []struct {
		name, method, path, accept string
		wantStatus                 int
		wantBody                   string
	}{
		{"root serves index", "GET", "/", "", 200, "<html>app</html>"},
		{"hashed asset served", "GET", "/_expo/static/app.js", "", 200, "console.log(1)"},
		{"unknown nav -> index shell", "GET", "/player", "text/html", 200, "<html>app</html>"},
		{"missing asset -> 404", "GET", "/assets/missing.png", "image/png", 404, ""},
		{"stray api path -> 404 not html", "GET", "/api/v1/bogus", "application/json", 404, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, nil)
			if c.accept != "" {
				req.Header.Set("Accept", c.accept)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, c.wantStatus)
			}
			if c.wantBody != "" && rr.Body.String() != c.wantBody {
				t.Fatalf("body = %q, want %q", rr.Body.String(), c.wantBody)
			}
		})
	}
}

// Placeholder-only (no index.html) must 404 so it never shadows the API.
func TestHandlerNoBuild(t *testing.T) {
	h := handler(fstest.MapFS{".gitkeep": {Data: []byte("x")}})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}
