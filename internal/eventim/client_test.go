package eventim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestSearchIsFranceOnlyAndFiltersFuzzyMatches covers that Search no-ops for
// any country but FR, and that product groups whose main attraction doesn't
// actually mention the artist (Eventim's search_term is a loose match) get
// filtered out.
func TestSearchIsFranceOnlyAndFiltersFuzzyMatches(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"productGroups":[
			{
				"name": "Jaÿ-Z 30",
				"mainAttraction": {"name": "Jay-Z"},
				"products": [{
					"productId": "21796403",
					"link": "https://www.eventim.fr/event/jay-z-30-stade-de-france-21796403/",
					"typeAttributes": {"liveEntertainment": {
						"location": {"city": "Paris", "name": "Stade De France"},
						"startDate": "2026-09-10T19:00:00+02:00"
					}}
				}]
			},
			{
				"name": "Ninon Valder",
				"mainAttraction": {"name": "Ninon Valder"},
				"products": [{
					"productId": "999",
					"link": "https://www.eventim.fr/event/ninon-valder-999/",
					"typeAttributes": {"liveEntertainment": {
						"location": {"city": "Lyon", "name": "Some Venue"},
						"startDate": "2026-09-11T19:00:00+02:00"
					}}
				}]
			}
		]}`))
	}))
	defer srv.Close()

	orig := baseURL
	baseURL = srv.URL
	defer func() { baseURL = orig }()

	c := NewClient()
	events, err := c.Search(context.Background(), "Jay-Z", "FR", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != "21796403" || events[0].City != "Paris" {
		t.Fatalf("Search = %+v, want only the real Jay-Z match", events)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("search_term") != "Jay-Z" {
		t.Fatalf("query = %q, want search_term=Jay-Z", gotQuery)
	}

	events, err = c.Search(context.Background(), "Jay-Z", "GB", 3)
	if err != nil || events != nil {
		t.Fatalf("Search(GB) = %v, %v, want nil, nil (France-only)", events, err)
	}
}

// TestMatchesArtistRequiresWholeWord is a regression test: a plain substring
// check let "Toto" false-match the unrelated attraction "ElGrandeToto".
func TestMatchesArtistRequiresWholeWord(t *testing.T) {
	cases := []struct {
		name, artist string
		want         bool
	}{
		{"ElGrandeToto - Salgoat World Tour", "Toto", false},
		{"Toto Live", "Toto", true},
		{"An Evening With Toto", "Toto", true},
		{"Jaÿ-Z 30", "Jay-Z", false}, // diaeresis: not the same word
		{"Ninho - Quattro Tour", "Ninho", true},
	}
	for _, c := range cases {
		if got := matchesArtist(c.name, c.artist); got != c.want {
			t.Errorf("matchesArtist(%q, %q) = %v, want %v", c.name, c.artist, got, c.want)
		}
	}
}
