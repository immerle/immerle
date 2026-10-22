package importer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeezerPlaylistID(t *testing.T) {
	cases := map[string]string{
		"https://www.deezer.com/en/playlist/908622995": "908622995",
		"https://www.deezer.com/playlist/123?utm=x":    "123",
		"908622995":                   "908622995",
		"https://deezer.com/track/55": "", // not a playlist
		"":                            "",
	}
	for in, want := range cases {
		if got := deezerPlaylistID(in); got != want {
			t.Errorf("deezerPlaylistID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeezerFetchPlaylistPaginates(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist/42":
			_, _ = w.Write([]byte(`{"title":"Roadtrip","tracks":{"data":[` +
				`{"title":"One","artist":{"name":"A"},"album":{"title":"X"}}],` +
				`"next":"` + srv.URL + `/playlist/42/tracks?index=1"}}`))
		case "/playlist/42/tracks":
			_, _ = w.Write([]byte(`{"data":[{"title":"Two","artist":{"name":"B"},"album":{"title":"Y"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := &deezerSource{client: srv.Client(), base: srv.URL}
	pl, err := d.FetchPlaylist(context.Background(), "https://www.deezer.com/playlist/42")
	if err != nil {
		t.Fatal(err)
	}
	if pl.Name != "Roadtrip" {
		t.Errorf("name = %q, want Roadtrip", pl.Name)
	}
	if len(pl.Tracks) != 2 || pl.Tracks[0].Title != "One" || pl.Tracks[1].Artist != "B" {
		t.Fatalf("tracks = %+v", pl.Tracks)
	}
}

func TestDeezerFetchPlaylistPropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"message":"Playlist not found"}}`))
	}))
	defer srv.Close()

	d := &deezerSource{client: srv.Client(), base: srv.URL}
	if _, err := d.FetchPlaylist(context.Background(), "999"); err == nil {
		t.Fatal("expected the deezer error to propagate")
	}
}
