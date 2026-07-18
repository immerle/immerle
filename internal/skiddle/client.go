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

// Search finds upcoming events matching artist in countryCode (an ISO
// 3166-1 alpha-2 code, e.g. "FR"), soonest first, capped at limit. Returns no
// events (not an error) when the client has no API key or nothing matches.
//
// Skiddle's keyword search is a loose, tokenized match (e.g. "Jay-Z" can
// match an unrelated event whose description merely contains the word
// "Jay") — results are filtered to those whose event name actually mentions
// the artist to avoid false positives.
func (c *Client) Search(ctx context.Context, artist, countryCode string, limit int) ([]Event, error) {
	if !c.IsConfigured() {
		return nil, nil
	}
	q := url.Values{
		"api_key": {c.apiKey},
		"keyword": {artist},
		"order":   {"date"},
		"limit":   {strconv.Itoa(limit)},
		"minDate": {time.Now().Format("2006-01-02")},
	}
	if countryCode != "" {
		q.Set("country", strings.ToUpper(countryCode))
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
		if !strings.Contains(strings.ToLower(name), strings.ToLower(artist)) {
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
