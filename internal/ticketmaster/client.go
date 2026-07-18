// Package ticketmaster searches the Ticketmaster Discovery API for upcoming
// music events by artist and city. Requires an API key (admin-configured,
// see models.ConcertsRuntime) — the free tier is plenty for a self-hosted
// instance's own concert-discovery sync.
package ticketmaster

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://app.ticketmaster.com/discovery/v2/events.json"

// Event is a single upcoming show, trimmed to what internal/concerts needs.
type Event struct {
	ID        string
	Name      string
	URL       string
	Venue     string
	City      string
	StartTime time.Time
}

// Client searches Ticketmaster's Discovery API. The zero value is not usable;
// build one with NewClient.
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

type discoveryResponse struct {
	Embedded struct {
		Events []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			URL   string `json:"url"`
			Dates struct {
				Start struct {
					DateTime  string `json:"dateTime"`
					LocalDate string `json:"localDate"`
				} `json:"start"`
			} `json:"dates"`
			Embedded struct {
				Venues []struct {
					Name string `json:"name"`
					City struct {
						Name string `json:"name"`
					} `json:"city"`
				} `json:"venues"`
			} `json:"_embedded"`
		} `json:"events"`
	} `json:"_embedded"`
}

// Search finds upcoming music events matching artist near city, soonest
// first, capped at limit. Returns no events (not an error) when the client
// has no API key or the artist has nothing upcoming.
//
// Ticketmaster's own `city` filter is tried first (precise, and cheap when it
// works), but it can miss a real match: we only have a free-text city with no
// country hint, so "Paris" is ambiguous to their API, and a venue's
// registered city doesn't always match common usage (an arena that's
// technically in a suburb). When the city-scoped search comes back empty, we
// retry without it and filter the broader result ourselves by a loose
// substring match on the venue's own city — a match we can verify precisely
// once we can see the actual event data, unlike the opaque server-side filter.
func (c *Client) Search(ctx context.Context, artist, city string, limit int) ([]Event, error) {
	if !c.IsConfigured() {
		return nil, nil
	}
	events, err := c.search(ctx, artist, city, limit)
	if err != nil {
		return nil, err
	}
	if len(events) > 0 || city == "" {
		return events, nil
	}
	broader, err := c.search(ctx, artist, "", limit*4)
	if err != nil {
		return nil, err
	}
	return filterByCity(broader, city, limit), nil
}

func (c *Client) search(ctx context.Context, artist, city string, limit int) ([]Event, error) {
	q := url.Values{
		"apikey":             {c.apiKey},
		"keyword":            {artist},
		"classificationName": {"music"},
		"sort":               {"date,asc"},
		"size":               {fmt.Sprintf("%d", limit)},
	}
	if city != "" {
		q.Set("city", city)
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
	// A city with zero matching events is a 200 with no "_embedded" key, not an
	// error — only treat non-2xx as a real failure.
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ticketmaster: unexpected status %d", resp.StatusCode)
	}
	var body discoveryResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(body.Embedded.Events))
	for _, e := range body.Embedded.Events {
		start, err := parseStart(e.Dates.Start.DateTime, e.Dates.Start.LocalDate)
		if err != nil || e.ID == "" {
			continue
		}
		ev := Event{ID: e.ID, Name: e.Name, URL: e.URL, StartTime: start}
		if len(e.Embedded.Venues) > 0 {
			ev.Venue = e.Embedded.Venues[0].Name
			ev.City = e.Embedded.Venues[0].City.Name
		}
		out = append(out, ev)
	}
	return out, nil
}

// filterByCity keeps events whose venue city loosely matches want (either
// contains the other, case-insensitive) — a substring match survives common
// variants ("Paris" vs "Paris La Défense") that a strict equality check
// wouldn't. An event with no venue city at all is dropped: we can't verify
// it's actually nearby, and showing an unrelated city defeats the feature.
func filterByCity(events []Event, want string, limit int) []Event {
	want = strings.ToLower(strings.TrimSpace(want))
	out := make([]Event, 0, limit)
	for _, e := range events {
		got := strings.ToLower(strings.TrimSpace(e.City))
		if got == "" {
			continue
		}
		if !strings.Contains(got, want) && !strings.Contains(want, got) {
			continue
		}
		out = append(out, e)
		if len(out) == limit {
			break
		}
	}
	return out
}

// parseStart prefers the precise dateTime (UTC, has a time-of-day);
// Ticketmaster omits it for a handful of events with only a date announced,
// in which case localDate (midnight) is the best available fallback.
func parseStart(dateTime, localDate string) (time.Time, error) {
	if dateTime != "" {
		return time.Parse(time.RFC3339, dateTime)
	}
	if localDate != "" {
		return time.Parse("2006-01-02", localDate)
	}
	return time.Time{}, fmt.Errorf("ticketmaster: event has no date")
}
