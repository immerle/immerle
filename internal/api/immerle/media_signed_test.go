package immerle

import (
	"net/http"
	"strings"
	"testing"
)

func TestSignedStreamURLs(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK || len(search.Songs()) == 0 {
		t.Fatalf("search: status %d, songs %d", st, len(search.Songs()))
	}
	id := search.Songs()[0].ID

	// Mint signed URLs (Bearer).
	var urls streamURLs
	if st := getJSON(t, srv, token, "/songs/"+id+"/stream-url", &urls); st != http.StatusOK {
		t.Fatalf("mint: status %d", st)
	}
	if !strings.Contains(urls.Stream, "/songs/"+id+"/stream?exp=") || !strings.Contains(urls.Stream, "&sig=") {
		t.Fatalf("unexpected signed url: %q", urls.Stream)
	}

	// The signed URL streams with NO Authorization header.
	resp, err := http.Get(srv.URL + urls.Stream)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signed stream: expected 200, got %d", resp.StatusCode)
	}

	// A tampered signature is rejected (401, no Bearer fallback).
	resp2, err := http.Get(srv.URL + urls.Stream + "0")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tampered sig: expected 401, got %d", resp2.StatusCode)
	}
}

func TestCoverIsPublic(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK || len(search.Songs()) == 0 {
		t.Fatalf("search: status %d", st)
	}
	albumID := search.Songs()[0].AlbumID

	// No Authorization header: a public route returns 404 for missing cover, NOT
	// 401 (which an authenticated route would return).
	resp, err := http.Get(srv.URL + apiBase + "/cover/" + albumID)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatal("cover route is not public (got 401)")
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing cover, got %d", resp.StatusCode)
	}
}
