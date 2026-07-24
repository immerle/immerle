package lrclib

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGetExactMatchPrefersSyncedOverPlain(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"plainLyrics":"plain","syncedLyrics":"[00:01.00]synced"}`))
	}))
	defer srv.Close()

	orig := getURL
	getURL = srv.URL
	defer func() { getURL = orig }()

	got, err := NewClient().Get(context.Background(), "Artist", "Title", "Album", 210)
	if err != nil {
		t.Fatal(err)
	}
	if got != "[00:01.00]synced" {
		t.Fatalf("Get = %q, want synced lyrics", got)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("artist_name") != "Artist" || q.Get("track_name") != "Title" || q.Get("album_name") != "Album" || q.Get("duration") != "210" {
		t.Fatalf("query = %q, unexpected params", gotQuery)
	}
}

// TestGetFallsBackToSearchOnExactMiss covers the Baby Shark case: the exact
// /get endpoint 404s (album/duration tags don't match lrclib's), but /search
// has several loosely-tagged releases — the closest by duration should win.
func TestGetFallsBackToSearchOnExactMiss(t *testing.T) {
	getSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer getSrv.Close()

	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"artistName":"Pinkfong","trackName":"Baby Shark","duration":106,"syncedLyrics":"far"},
			{"artistName":"Pinkfong","trackName":"Baby Shark","duration":96,"syncedLyrics":"close"},
			{"artistName":"Pinkfong","trackName":"Baby Shark","duration":80,"instrumental":true,"syncedLyrics":"skip-instrumental"}
		]`))
	}))
	defer searchSrv.Close()

	origGet, origSearch := getURL, searchURL
	getURL, searchURL = getSrv.URL, searchSrv.URL
	defer func() { getURL, searchURL = origGet, origSearch }()

	got, err := NewClient().Get(context.Background(), "Pinkfong", "Baby Shark", "", 97)
	if err != nil {
		t.Fatal(err)
	}
	if got != "close" {
		t.Fatalf("Get = %q, want the closest-duration, non-instrumental match", got)
	}
}

func TestGetNoMatchAnywhereReturnsEmptyNoError(t *testing.T) {
	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer notFound.Close()

	origGet, origSearch := getURL, searchURL
	getURL, searchURL = notFound.URL, notFound.URL
	defer func() { getURL, searchURL = origGet, origSearch }()

	got, err := NewClient().Get(context.Background(), "Artist", "Title", "", 0)
	if err != nil || got != "" {
		t.Fatalf("Get(no match) = %q, %v, want \"\", nil", got, err)
	}
}
