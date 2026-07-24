package ticketmaster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestSearchSendsCountryCodeAndParsesEvents covers the actual query Search
// sends (countryCode, not a free-text city — see the package doc for why)
// and that a real Discovery API response shape gets parsed correctly.
func TestSearchSendsCountryCodeAndParsesEvents(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"_embedded":{"events":[{
			"id": "evt-1",
			"name": "Jay-Z World Tour",
			"url": "https://example.com/evt-1",
			"dates": {"start": {"dateTime": "2026-09-12T19:00:00Z"}},
			"_embedded": {"venues": [{"name": "Accor Arena", "city": {"name": "Berlin"}}]}
		}]}}`))
	}))
	defer srv.Close()

	orig := baseURL
	baseURL = srv.URL
	defer func() { baseURL = orig }()

	c := NewClient("test-key")
	events, err := c.Search(context.Background(), "Jay-Z", "DE", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != "evt-1" || events[0].City != "Berlin" || events[0].Venue != "Accor Arena" {
		t.Fatalf("Search = %+v, want one parsed event", events)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("countryCode") != "DE" {
		t.Fatalf("query = %q, want countryCode=DE", gotQuery)
	}
	if q.Get("keyword") != "Jay-Z" {
		t.Fatalf("query = %q, want keyword=Jay-Z", gotQuery)
	}
}

func TestSearchNoOpWithoutAPIKey(t *testing.T) {
	c := NewClient("")
	events, err := c.Search(context.Background(), "Jay-Z", "DE", 3)
	if err != nil || events != nil {
		t.Fatalf("Search(no key) = %v, %v, want nil, nil", events, err)
	}
}

// TestSearchNoOpOutsideSupportedCountries is a regression test: Ticketmaster's
// Discovery API has almost no French catalog for this key (verified live: 1
// total music event for FR vs. 1000+ for most other markets), so France
// isn't in supportedCountries — see internal/eventim for the France-specific
// replacement source.
func TestSearchNoOpOutsideSupportedCountries(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{}`))
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
