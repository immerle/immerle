// Package skiddle searches the Skiddle events API for upcoming shows by
// keyword. Used as the fallback to Ticketmaster (see internal/concerts) —
// Skiddle's catalog skews UK/Europe, and its search only takes a free-text
// keyword (no artist-specific field), so matches here are approximate:
// artist name and city combined into one keyword.
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

const baseURL = "https://www.skiddle.com/api/v1/events/search/"

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

// Search finds upcoming events matching a free-text combination of artist and
// city, soonest first, capped at limit. Returns no events (not an error) when
// the client has no API key or nothing matches.
func (c *Client) Search(ctx context.Context, artist, city string, limit int) ([]Event, error) {
	if !c.IsConfigured() {
		return nil, nil
	}
	keyword := artist
	if city != "" {
		keyword = artist + " " + city
	}
	q := url.Values{
		"api_key": {c.apiKey},
		"keyword": {keyword},
		"order":   {"date"},
		"limit":   {strconv.Itoa(limit)},
		"minDate": {time.Now().Format("2006-01-02")},
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
		out = append(out, Event{
			ID:        e.ID.String(),
			Name:      strings.TrimSpace(e.Name),
			URL:       e.Link,
			Venue:     e.Venue.Name,
			City:      e.Venue.Town,
			StartTime: start,
		})
	}
	return out, nil
}
