// Package reccobeats calls the keyless ReccoBeats API
// (https://reccobeats.com/docs/apis/reccobeats-api) to turn a handful of seed
// tracks into similar-track recommendations. ReccoBeats has no idea what's in
// any given self-hosted library — it only knows Spotify-side metadata — so it
// resolves seeds by searching its own catalog for artist+title text, and
// returns recommendations the same way (artist+title+ISRC), left for the
// caller to match back against its own local tracks.
package reccobeats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/providers"
)

const baseURL = "https://api.reccobeats.com"

// maxSeeds is ReccoBeats' hard limit on /v1/track/recommendation's seeds param.
const maxSeeds = 5

// Client calls the ReccoBeats API. The zero value is not usable; build one
// with NewClient. Safe for concurrent use (holds no mutable state).
type Client struct {
	http *http.Client
}

// NewClient builds a Client. No API key is needed or supported.
func NewClient() *Client {
	return &Client{http: providers.NewHTTPClient(15 * time.Second)}
}

// Seed identifies a track to base recommendations on, by portable
// artist/title (a local library track's own metadata).
type Seed struct {
	Artist string
	Title  string
}

// Track is a recommended track, identified the same portable way a Seed is —
// callers match it back to their own catalog (e.g.
// persistence.CatalogRepo.FindByArtistTitle).
type Track struct {
	Artist string
	Title  string
	ISRC   string
}

// Recommend resolves up to maxSeeds of the given seeds to ReccoBeats track
// ids (searching its catalog by artist+title text; a seed with no match is
// skipped) and returns up to size recommended tracks based on them. Fails
// only if none of the seeds could be resolved.
func (c *Client) Recommend(ctx context.Context, seeds []Seed, size int) ([]Track, error) {
	var ids []string
	for _, s := range seeds {
		if len(ids) >= maxSeeds {
			break
		}
		id, err := c.searchID(ctx, s.Artist, s.Title)
		if err != nil {
			return nil, err
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("reccobeats: none of the %d seed(s) matched a ReccoBeats track", len(seeds))
	}
	return c.recommend(ctx, ids, size)
}

// searchID looks up the ReccoBeats id of the best text search hit for title.
// Deliberately title-only: ReccoBeats' full-text search breaks down (0 hits)
// far more often when the artist name is folded into the same searchText —
// verified by hand across several real seeds (e.g. "Damso Θ. Macarena" → 0
// hits, "Θ. Macarena" alone → the exact track). Title-only search also
// doesn't rank by popularity or artist, so a well-known track can still lose
// to an obscure cover (also verified: "Get Lucky" ranked a jazz cover first,
// the real Daft Punk recording absent even 250 results in) — so this
// requires an exact (case-insensitive) artist match among the results,
// breaking ties by the highest popularity score, and returns "" (skip this
// seed) if no candidate credits the wanted artist at all.
func (c *Client) searchID(ctx context.Context, artist, title string) (string, error) {
	var out struct {
		Content []searchTrack `json:"content"`
	}
	q := url.Values{"searchText": {strings.TrimSpace(title)}}
	if err := c.get(ctx, "/v1/track/search", q, &out); err != nil {
		return "", err
	}
	best := -1
	for i, t := range out.Content {
		if !hasArtist(t.Artists, artist) {
			continue
		}
		if best == -1 || t.Popularity > out.Content[best].Popularity {
			best = i
		}
	}
	if best == -1 {
		return "", nil
	}
	return out.Content[best].ID, nil
}

type searchTrack struct {
	ID         string         `json:"id"`
	Popularity int            `json:"popularity"`
	Artists    []searchArtist `json:"artists"`
}

type searchArtist struct {
	Name string `json:"name"`
}

func hasArtist(artists []searchArtist, want string) bool {
	for _, a := range artists {
		if strings.EqualFold(a.Name, want) {
			return true
		}
	}
	return false
}

// recommend fetches recommendations for already-resolved ReccoBeats seed ids.
func (c *Client) recommend(ctx context.Context, seedIDs []string, size int) ([]Track, error) {
	var out struct {
		Content []struct {
			TrackTitle string `json:"trackTitle"`
			ISRC       string `json:"isrc"`
			Artists    []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"content"`
	}
	q := url.Values{
		"seeds": {strings.Join(seedIDs, ",")},
		"size":  {fmt.Sprint(size)},
	}
	if err := c.get(ctx, "/v1/track/recommendation", q, &out); err != nil {
		return nil, err
	}
	tracks := make([]Track, 0, len(out.Content))
	for _, t := range out.Content {
		if len(t.Artists) == 0 || t.TrackTitle == "" {
			continue
		}
		tracks = append(tracks, Track{Artist: t.Artists[0].Name, Title: t.TrackTitle, ISRC: t.ISRC})
	}
	return tracks, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("reccobeats: %s: unexpected status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
