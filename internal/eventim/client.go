// Package eventim searches Eventim/France Billet's public product-search API
// for upcoming concerts. Unlike Ticketmaster and Skiddle, it needs no API
// key — but its catalog is France-only, so Search is a no-op for any other
// country (see internal/concerts, which chains it in after Ticketmaster and
// Skiddle as a France-specific extra source).
package eventim

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
var baseURL = "https://public-api.eventim.com/websearch/search/api/exploration/v2/productGroups"

// Event is a single upcoming show, trimmed to what internal/concerts needs.
type Event struct {
	ID        string
	Name      string
	URL       string
	Venue     string
	City      string
	StartTime time.Time
}

// Client searches Eventim's France storefront. The zero value is usable —
// there's no API key.
type Client struct{ http *http.Client }

// NewClient builds a Client.
func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}}
}

type productGroupsResponse struct {
	ProductGroups []struct {
		Name           string `json:"name"`
		MainAttraction struct {
			Name string `json:"name"`
		} `json:"mainAttraction"`
		Products []struct {
			ProductID      string `json:"productId"`
			Link           string `json:"link"`
			TypeAttributes struct {
				LiveEntertainment struct {
					Location struct {
						City string `json:"city"`
						Name string `json:"name"`
					} `json:"location"`
					StartDate string `json:"startDate"`
				} `json:"liveEntertainment"`
			} `json:"typeAttributes"`
		} `json:"products"`
	} `json:"productGroups"`
}

// Search finds upcoming concerts matching artist, soonest first, capped at
// limit. countryCode must be "FR" (case-insensitive) — Eventim's public
// storefront only covers France, so any other country returns no events (not
// an error), same as an unconfigured Ticketmaster/Skiddle client.
//
// Eventim's search_term is a loose match on the whole product group (e.g.
// searching "Ninho" also surfaces "Ninon Valder", "Nino Gotfunk"...), so
// results are filtered to those whose main attraction name actually mentions
// the artist.
func (c *Client) Search(ctx context.Context, artist, countryCode string, limit int) ([]Event, error) {
	if !strings.EqualFold(countryCode, "FR") {
		return nil, nil
	}
	q := url.Values{
		"webId":          {"web__eventim-fr"},
		"language":       {"fr"},
		"page":           {"1"},
		"retail_partner": {"1FR"},
		"categories":     {"Concerts & Festivals"},
		"sort":           {"DateAsc"},
		"tags":           {"DISABLE_FBS"},
		"search_term":    {artist},
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
		return nil, fmt.Errorf("eventim: unexpected status %d", resp.StatusCode)
	}
	var body productGroupsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	var out []Event
	for _, pg := range body.ProductGroups {
		if !strings.Contains(strings.ToLower(pg.MainAttraction.Name), strings.ToLower(artist)) {
			continue
		}
		for _, p := range pg.Products {
			start, err := time.Parse(time.RFC3339, p.TypeAttributes.LiveEntertainment.StartDate)
			if err != nil || p.ProductID == "" {
				continue
			}
			out = append(out, Event{
				ID:        p.ProductID,
				Name:      pg.Name,
				URL:       p.Link,
				Venue:     p.TypeAttributes.LiveEntertainment.Location.Name,
				City:      p.TypeAttributes.LiveEntertainment.Location.City,
				StartTime: start,
			})
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}
