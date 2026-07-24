package musicbrainz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLookupByISRC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/isrc/USRC17607839" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"isrc":"USRC17607839","recordings":[{"id":"b9b45f8b-1d0f-4e2a-9d5f-000000000001"}]}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	mbid, err := c.LookupByISRC(context.Background(), "USRC17607839")
	if err != nil {
		t.Fatal(err)
	}
	if mbid != "b9b45f8b-1d0f-4e2a-9d5f-000000000001" {
		t.Errorf("mbid = %q", mbid)
	}
}

func TestLookupByISRCNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	mbid, err := c.LookupByISRC(context.Background(), "ZZXXX0000000")
	if err != nil {
		t.Fatal(err)
	}
	if mbid != "" {
		t.Errorf("mbid = %q, want empty", mbid)
	}
}

func TestSearchRecording(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/recording" {
			t.Errorf("path = %q", r.URL.Path)
		}
		gotQuery = r.URL.Query().Get("query")
		w.Write([]byte(`{"recordings":[
			{"id":"mbid-1","title":"Get Lucky","artist-credit":[{"name":"Daft Punk"}],"releases":[{"title":"Discovery"}]},
			{"id":"","title":"skip me (no id)"}
		]}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	candidates, err := c.SearchRecording(context.Background(), "Daft Punk", "Get Lucky")
	if err != nil {
		t.Fatal(err)
	}
	wantQuery := `recording:"Get Lucky" AND artist:"Daft Punk"`
	if gotQuery != wantQuery {
		t.Errorf("query = %q, want %q", gotQuery, wantQuery)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate (the one with no id skipped), got %d", len(candidates))
	}
	got := candidates[0]
	if got.MBID != "mbid-1" || got.Title != "Get Lucky" || got.ArtistName != "Daft Punk" || got.AlbumName != "Discovery" {
		t.Errorf("candidate = %+v", got)
	}
}

func TestLookupISRC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/recording/b9b45f8b-1d0f-4e2a-9d5f-000000000001" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("inc") != "isrcs" {
			t.Errorf("inc = %q, want isrcs", r.URL.Query().Get("inc"))
		}
		w.Write([]byte(`{"id":"b9b45f8b-1d0f-4e2a-9d5f-000000000001","isrcs":["USRC17607839"]}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	isrc, err := c.LookupISRC(context.Background(), "b9b45f8b-1d0f-4e2a-9d5f-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if isrc != "USRC17607839" {
		t.Errorf("isrc = %q", isrc)
	}
}

func TestLookupISRCNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"x","isrcs":[]}`))
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	isrc, err := c.LookupISRC(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if isrc != "" {
		t.Errorf("isrc = %q, want empty", isrc)
	}
}

func TestThrottleSerializesRequests(t *testing.T) {
	c := newClient("http://unused.invalid", nil)
	c.next = time.Now().Add(minInterval) // simulate a request made just now

	start := time.Now()
	if err := c.throttle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < minInterval-10*time.Millisecond {
		t.Errorf("throttle returned after %v, want at least %v", elapsed, minInterval)
	}
}
