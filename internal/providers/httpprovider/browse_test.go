package httpprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/immerle/immerle/internal/providers"
)

// A generic HTTP provider advertises every browse capability (it degrades to
// empty results when the remote service doesn't implement an endpoint).
var (
	_ providers.ArtistSearcher      = (*Provider)(nil)
	_ providers.ArtistAlbumLister   = (*Provider)(nil)
	_ providers.ArtistBrowser       = (*Provider)(nil)
	_ providers.AlbumBrowser        = (*Provider)(nil)
	_ providers.ArtistImageSearcher = (*Provider)(nil)
	_ providers.PlaylistBrowser     = (*Provider)(nil)
)

func browseService(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/artists", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"artists":[{"providerArtistId":"a1","name":"Artist","albumCount":3,"imageUrl":"http://img/a"}]}`))
	})
	mux.HandleFunc("/artist/albums", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "a1" {
			http.Error(w, "no", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"albums":[{"providerAlbumId":"al1","title":"Album","year":2020,"coverImageUrl":"http://img/c"}]}`))
	})
	mux.HandleFunc("/album/tracks", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"providerTrackId":"t1","title":"Song","artist":"Artist","suffix":"mp3"}]}`))
	})
	mux.HandleFunc("/artist/image", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "Artist" {
			http.Error(w, "no", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"imageUrl":"http://img/avatar.jpg"}`))
	})
	mux.HandleFunc("/playlists", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"playlists":[{"providerPlaylistId":"p1","name":"Chill","coverImageUrl":"http://img/p",
			"tracks":[{"providerTrackId":"t1","title":"Song","artist":"Artist","suffix":"mp3"},{"providerTrackId":"","title":"skip me"}]}]}`))
	})
	// Note: /artist/tracks intentionally NOT implemented → 404 → graceful empty.
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPProviderBrowse(t *testing.T) {
	srv := browseService(t)
	p, err := New("deezer", srv.URL, "{}")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	artists, err := p.SearchArtists(ctx, "Artist", 10)
	if err != nil || len(artists) != 1 || artists[0].ProviderArtistID != "a1" || artists[0].AlbumCount != 3 {
		t.Fatalf("SearchArtists: %+v err=%v", artists, err)
	}

	albums, err := p.ArtistAlbums(ctx, "a1", 100)
	if err != nil || len(albums) != 1 || albums[0].ProviderAlbumID != "al1" || albums[0].Year != 2020 {
		t.Fatalf("ArtistAlbums: %+v err=%v", albums, err)
	}

	tracks, err := p.AlbumTracks(ctx, "al1", 200)
	if err != nil || len(tracks) != 1 || tracks[0].ProviderTrackID != "t1" {
		t.Fatalf("AlbumTracks: %+v err=%v", tracks, err)
	}

	img, err := p.ArtistImage(ctx, "Artist")
	if err != nil || img != "http://img/avatar.jpg" {
		t.Fatalf("ArtistImage: %q err=%v", img, err)
	}

	// Unknown artist (404) degrades to an empty image, not an error.
	img, err = p.ArtistImage(ctx, "Nobody")
	if err != nil {
		t.Fatalf("ArtistImage on a 404 should not error: %v", err)
	}
	if img != "" {
		t.Fatalf("expected empty image for unknown artist, got %q", img)
	}

	// Unimplemented endpoint (404) degrades to an empty result, not an error.
	top, err := p.ArtistTracks(ctx, "a1", 50)
	if err != nil {
		t.Fatalf("ArtistTracks on a 404 endpoint should not error: %v", err)
	}
	if len(top) != 0 {
		t.Fatalf("expected empty top tracks, got %d", len(top))
	}

	playlists, err := p.Playlists(ctx, 10)
	if err != nil || len(playlists) != 1 {
		t.Fatalf("Playlists: %+v err=%v", playlists, err)
	}
	pl := playlists[0]
	if pl.ProviderPlaylistID != "p1" || pl.Name != "Chill" || pl.CoverImageURL != "http://img/p" {
		t.Fatalf("unexpected playlist: %+v", pl)
	}
	if len(pl.Tracks) != 1 || pl.Tracks[0].ProviderTrackID != "t1" { // the empty-id row is dropped
		t.Fatalf("unexpected playlist tracks: %+v", pl.Tracks)
	}
}

// TestHTTPProviderPlaylistsUnsupported exercises the graceful-404 path: a
// remote that doesn't implement /playlists degrades to an empty result.
func TestHTTPProviderPlaylistsUnsupported(t *testing.T) {
	bare := httptest.NewServer(http.NewServeMux())
	t.Cleanup(bare.Close)
	p, err := New("deezer", bare.URL, "{}")
	if err != nil {
		t.Fatal(err)
	}
	playlists, err := p.Playlists(context.Background(), 10)
	if err != nil {
		t.Fatalf("Playlists on a 404 endpoint should not error: %v", err)
	}
	if len(playlists) != 0 {
		t.Fatalf("expected empty playlists, got %d", len(playlists))
	}
}
