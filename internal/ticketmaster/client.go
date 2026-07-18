// Package ticketmaster searches the Ticketmaster Discovery API for upcoming
// music events by artist and country. Requires an API key (admin-configured,
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

// baseURL is a var, not a const, so tests can point it at an httptest server.
var baseURL = "https://app.ticketmaster.com/discovery/v2/events.json"

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

// supportedCountries are the markets where Ticketmaster's Discovery API
// actually has a usable music catalog — checked live against every country
// offered by the admin dropdown (ui/src/utils/countries.ts): a "music"
// classification search returned >=100 total events for each of these, and
// single digits or zero for everything else (e.g. France: 1 total event —
// see the France-specific Eventim source in internal/eventim for why).
// Countries outside this list are a no-op, same as an unconfigured client.
var supportedCountries = map[string]bool{
	"US": true, "GB": true, "DE": true, "ES": true, "IT": true, "NL": true,
	"BE": true, "IE": true, "CA": true, "AU": true, "NZ": true, "SE": true,
	"NO": true, "DK": true, "FI": true, "PL": true, "AT": true, "CH": true,
	"MX": true, "BR": true, "TR": true, "CZ": true,
}

// Search finds upcoming music events matching artist in countryCode (an ISO
// 3166-1 alpha-2 code, e.g. "FR"), soonest first, capped at limit. Returns no
// events (not an error) when the client has no API key, countryCode isn't in
// supportedCountries, or the artist has nothing upcoming there.
func (c *Client) Search(ctx context.Context, artist, countryCode string, limit int) ([]Event, error) {
	if !c.IsConfigured() || !supportedCountries[strings.ToUpper(countryCode)] {
		return nil, nil
	}
	q := url.Values{
		"apikey":             {c.apiKey},
		"keyword":            {artist},
		"classificationName": {"music"},
		"sort":               {"date,asc"},
		"size":               {fmt.Sprintf("%d", limit)},
	}
	if countryCode != "" {
		q.Set("countryCode", countryCode)
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
	// A country with zero matching events is a 200 with no "_embedded" key,
	// not an error — only treat non-2xx as a real failure.
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
