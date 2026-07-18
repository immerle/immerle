// Package lrclib fetches lyrics from the free, keyless lrclib.net API, used
// as a fallback when a track has no embedded/sidecar lyrics.
package lrclib

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"
)

// getURL and searchURL are vars, not consts, so tests can point them at an
// httptest server.
var (
	getURL    = "https://lrclib.net/api/get"
	searchURL = "https://lrclib.net/api/search"
)

// Client fetches lyrics from lrclib.net.
type Client struct {
	http *http.Client
}

// NewClient builds a Client.
func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 10 * time.Second}}
}

type result struct {
	ArtistName   string  `json:"artistName"`
	TrackName    string  `json:"trackName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

func (r result) lyrics() string {
	if r.SyncedLyrics != "" {
		return r.SyncedLyrics
	}
	return r.PlainLyrics
}

// Get looks up lyrics for a track by artist/title/album/duration (seconds).
// It tries an exact match first, then falls back to lrclib's fuzzy search
// (real-world tags rarely have the exact album name or duration lrclib
// expects — e.g. "Baby Shark" alone has 20+ differently-tagged releases with
// durations from 81s to 106s). Returns "" (no error) when nothing matches;
// synced lyrics are preferred over plain when both are present.
func (c *Client) Get(ctx context.Context, artist, title, album string, durationSec int) (string, error) {
	if lyrics, err := c.exact(ctx, artist, title, album, durationSec); err != nil {
		return "", err
	} else if lyrics != "" {
		return lyrics, nil
	}
	return c.search(ctx, artist, title, durationSec)
}

func (c *Client) exact(ctx context.Context, artist, title, album string, durationSec int) (string, error) {
	q := url.Values{"artist_name": {artist}, "track_name": {title}}
	if album != "" {
		q.Set("album_name", album)
	}
	if durationSec > 0 {
		q.Set("duration", fmt.Sprintf("%d", durationSec))
	}
	var r result
	found, err := c.do(ctx, getURL+"?"+q.Encode(), &r)
	if err != nil || !found {
		return "", err
	}
	return r.lyrics(), nil
}

// search picks the fuzzy-search result closest in duration to durationSec
// (or the first result if durationSec is unknown) among candidates that
// actually have lyrics.
func (c *Client) search(ctx context.Context, artist, title string, durationSec int) (string, error) {
	q := url.Values{"artist_name": {artist}, "track_name": {title}}
	var results []result
	found, err := c.do(ctx, searchURL+"?"+q.Encode(), &results)
	if err != nil || !found {
		return "", err
	}
	best := -1
	bestDiff := math.MaxFloat64
	for i, r := range results {
		if r.Instrumental || r.lyrics() == "" {
			continue
		}
		if durationSec <= 0 {
			return r.lyrics(), nil
		}
		diff := math.Abs(r.Duration - float64(durationSec))
		if diff < bestDiff {
			best, bestDiff = i, diff
		}
	}
	if best < 0 {
		return "", nil
	}
	return results[best].lyrics(), nil
}

// do decodes the JSON response body from url into out. found is false (with
// no error) on a 404 — that's "no match", not a failure.
func (c *Client) do(ctx context.Context, url string, out any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= 300 {
		return false, fmt.Errorf("lrclib: unexpected status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return false, err
	}
	return true, nil
}
