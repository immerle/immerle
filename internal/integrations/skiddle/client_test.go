package skiddle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestSearchSendsCountryAndFiltersFuzzyMatches covers the actual query Search
// sends (a structured "country" param, not a free-text keyword — see the
// package doc for why) and that events whose name doesn't actually mention
// the artist (Skiddle's keyword search is a loose, tokenized match) get
// filtered out.
func TestSearchSendsCountryAndFiltersFuzzyMatches(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":0,"totalcount":"2","results":[
			{"id":"1","eventname":"Jay-Z Live","link":"https://example.com/1","date":"2026-09-12","venue":{"name":"O2 Arena","town":"London"}},
			{"id":"2","eventname":"Tony Jay Presents","link":"https://example.com/2","date":"2026-09-13","venue":{"name":"Some Bar","town":"Glasgow"}}
		]}`))
	}))
	defer srv.Close()

	orig := baseURL
	baseURL = srv.URL
	defer func() { baseURL = orig }()

	c := NewClient("test-key")
	events, err := c.Search(context.Background(), "Jay-Z", "GB", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != "1" || events[0].City != "London" {
		t.Fatalf("Search = %+v, want only the real Jay-Z match", events)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("country") != "GB" {
		t.Fatalf("query = %q, want country=GB", gotQuery)
	}
	if q.Get("keyword") != "Jay-Z" {
		t.Fatalf("query = %q, want keyword=Jay-Z", gotQuery)
	}
}

func TestSearchNoOpWithoutAPIKey(t *testing.T) {
	c := NewClient("")
	events, err := c.Search(context.Background(), "Jay-Z", "GB", 3)
	if err != nil || events != nil {
		t.Fatalf("Search(no key) = %v, %v, want nil, nil", events, err)
	}
}

// TestSearchNoOpOutsideSupportedCountries is a regression test: Skiddle's
// catalog outside its handful of strong markets is too thin to be useful
// (verified live against every country in the admin dropdown).
func TestSearchNoOpOutsideSupportedCountries(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"error":0,"totalcount":"0","results":[]}`))
	}))
	defer srv.Close()

	orig := baseURL
	baseURL = srv.URL
	defer func() { baseURL = orig }()

	c := NewClient("test-key")
	events, err := c.Search(context.Background(), "Jay-Z", "FR", 3)
	if err != nil || events != nil {
		t.Fatalf("Search(FR) = %v, %v, want nil, nil (unsupported country)", events, err)
	}
	if called {
		t.Fatal("Search(FR) made an HTTP request — unsupported countries must be a pure no-op")
	}
}

// TestMatchesArtistRequiresWholeWord is a regression test: a plain substring
// check let "Toto" false-match the unrelated event "ElGrandeToto".
func TestMatchesArtistRequiresWholeWord(t *testing.T) {
	cases := []struct {
		name, artist string
		want         bool
	}{
		{"ElGrandeToto - Salgoat World Tour", "Toto", false},
		{"Toto Live", "Toto", true},
		{"An Evening With Toto", "Toto", true},
		{"Tony Jay Presents", "Jay-Z", false},
	}
	for _, c := range cases {
		if got := matchesArtist(c.name, c.artist); got != c.want {
			t.Errorf("matchesArtist(%q, %q) = %v, want %v", c.name, c.artist, got, c.want)
		}
	}
}
