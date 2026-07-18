// Package skiddle searches the Skiddle events API for upcoming shows by
// keyword and country. Used as the fallback to Ticketmaster (see
// internal/concerts) — Skiddle's catalog skews UK/Europe. Its keyword search
// is a loose, tokenized match, so results are filtered to those whose event
// name actually mentions the artist.
package skiddle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// baseURL is a var, not a const, so tests can point it at an httptest server.
var baseURL = "https://www.skiddle.com/api/v1/events/search/"

// Event is a single upcoming show, trimmed to what internal/concerts needs.
type Event struct {
	ID        string
	Name      string
	URL       string
	Venue     string
	City      string
	StartTime time.Time
}

// Client searches Skiddle's events API. The zero value is not usable; build
// one with NewClient.
type Client struct {
	http   *http.Client
	apiKey string
}

// NewClient builds a Client. An empty apiKey makes every search a no-op
// (returns no events, no error) — callers don't need to check IsConfigured
// before calling Search.
func NewClient(apiKey string) *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}, apiKey: apiKey}
}

// IsConfigured reports whether an API key is set.
func (c *Client) IsConfigured() bool { return c.apiKey != "" }

type searchResponse struct {
	Results []struct {
		ID    json.Number `json:"id"`
		Name  string      `json:"eventname"`
		Link  string      `json:"link"`
		Date  string      `json:"date"` // "YYYY-MM-DD"
		Venue struct {
			Name string `json:"name"`
			Town string `json:"town"`
		} `json:"venue"`
	} `json:"results"`
}

// supportedCountries are the markets where Skiddle actually has a usable
// catalog — checked live against every country offered by the admin dropdown
// (ui/src/utils/countries.ts): a generic search returned close to 100 events
// or more for each of these, and single/low-double digits for everything
// else. Countries outside this list are a no-op, same as an unconfigured
// client.
var supportedCountries = map[string]bool{
	"GB": true, "IE": true, "ES": true, "GR": true, "PT": true,
}

// Search finds upcoming events matching artist in countryCode (an ISO
// 3166-1 alpha-2 code, e.g. "FR"), soonest first, capped at limit. Returns no
// events (not an error) when the client has no API key, countryCode isn't in
// supportedCountries, or nothing matches.
//
// Skiddle's keyword search is a loose, tokenized match (e.g. "Jay-Z" can
// match an unrelated event whose description merely contains the word
// "Jay") — results are filtered to those whose event name actually mentions
// the artist as a whole word, to avoid false positives (a plain substring
// check would let a short name like "Toto" false-match "ElGrandeToto").
func (c *Client) Search(ctx context.Context, artist, countryCode string, limit int) ([]Event, error) {
	countryCode = strings.ToUpper(countryCode)
	if !c.IsConfigured() || !supportedCountries[countryCode] {
		return nil, nil
	}
	q := url.Values{
		"api_key": {c.apiKey},
		"keyword": {artist},
		"order":   {"date"},
		"limit":   {strconv.Itoa(limit)},
		"minDate": {time.Now().Format("2006-01-02")},
		"country": {countryCode},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("skiddle: unexpected status %d", resp.StatusCode)
	}
	var body searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(body.Results))
	for _, e := range body.Results {
		if e.ID.String() == "" || e.Date == "" {
			continue
		}
		start, err := time.Parse("2006-01-02", e.Date)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(e.Name)
		if !matchesArtist(name, artist) {
			continue
		}
		out = append(out, Event{
			ID:        e.ID.String(),
			Name:      name,
			URL:       e.Link,
			Venue:     e.Venue.Name,
			City:      e.Venue.Town,
			StartTime: start,
		})
	}
	return out, nil
}

// matchesArtist reports whether artist appears in name as a whole word
// (case-insensitive) — a plain substring check would let a short/common
// artist name false-match inside an unrelated longer word.
func matchesArtist(name, artist string) bool {
	re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(artist) + `\b`)
	if err != nil {
		return false
	}
	return re.MatchString(name)
}
