// Package musicbrainz resolves the MusicBrainz recording id (MBID) of a
// track, by ISRC or by artist/title text search, so downloads/uploads with no
// embedded MusicBrainz tag can still get one -- e.g. for precise ListenBrainz
// scrobble linking.
package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/immerle/immerle/internal/models"
)

// defaultBaseURL is MusicBrainz's API root.
const defaultBaseURL = "https://musicbrainz.org/ws/2"

// minInterval throttles requests to MusicBrainz's documented limit for
// unauthenticated clients (about 1 request/second).
const minInterval = time.Second

// userAgent identifies the app per MusicBrainz's API etiquette
// (https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting).
const userAgent = "immerle/1.0 ( https://github.com/immerle/immerle )"

// maxResponseBytes caps an in-memory response body.
const maxResponseBytes = 1 << 20

// searchLimit caps how many recording candidates a text search returns --
// the caller (core.MusicBrainzEnricher) re-ranks them itself, so there's no
// value in MusicBrainz's own long tail.
const searchLimit = 10

// Client looks up recordings on the MusicBrainz API. Safe for concurrent use;
// requests are serialized and throttled to minInterval.
type Client struct {
	baseURL string
	http    *http.Client

	mu   sync.Mutex
	next time.Time // earliest time the next request may fire
}

// NewClient builds a Client against the real MusicBrainz API.
func NewClient() *Client {
	return newClient(defaultBaseURL, nil)
}

func newClient(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: hc}
}

// throttle blocks until minInterval has passed since the last request. next
// tracks the earliest time a request may fire; each call reserves the
// following slot so concurrent callers still serialize to one per interval.
func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	wait := time.Until(c.next)
	if wait < 0 {
		wait = 0
	}
	c.next = time.Now().Add(wait + minInterval)
	c.mu.Unlock()
	if wait == 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// get performs a throttled, rate-limit-compliant GET against the MusicBrainz
// API and decodes the JSON response into out.
func (c *Client) get(ctx context.Context, path string, out any) error {
	if err := c.throttle(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("musicbrainz: %s: unexpected status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return fmt.Errorf("musicbrainz: %s: decode: %w", path, err)
	}
	return nil
}

// LookupByISRC returns the MBID of the recording matching isrc, or "" if
// MusicBrainz has no recording for it. When several recordings share the ISRC
// (alternate masters/remasters), the first one MusicBrainz returns is used.
func (c *Client) LookupByISRC(ctx context.Context, isrc string) (string, error) {
	if isrc == "" {
		return "", nil
	}
	var out struct {
		Recordings []struct {
			ID string `json:"id"`
		} `json:"recordings"`
	}
	if err := c.get(ctx, "/isrc/"+url.PathEscape(isrc)+"?fmt=json", &out); err != nil {
		return "", err
	}
	if len(out.Recordings) == 0 {
		return "", nil
	}
	return out.Recordings[0].ID, nil
}

// SearchRecording searches MusicBrainz by artist+title text, for tracks with
// no ISRC to look up directly. It returns raw, unranked candidates (as
// portable models.Track values -- only MBID/Title/ArtistName/AlbumName are
// set): MusicBrainz's own full-text relevance isn't reliable enough to trust
// blindly (a well-known track can lose to an obscure cover), so disambiguation
// is left to the caller, exactly like a provider search result.
func (c *Client) SearchRecording(ctx context.Context, artist, title string) ([]models.Track, error) {
	title, artist = strings.TrimSpace(title), strings.TrimSpace(artist)
	if title == "" {
		return nil, nil
	}
	query := `recording:"` + escapeLucene(title) + `"`
	if artist != "" {
		query += ` AND artist:"` + escapeLucene(artist) + `"`
	}
	var out struct {
		Recordings []struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			ArtistCredit []struct {
				Name string `json:"name"`
			} `json:"artist-credit"`
			Releases []struct {
				Title string `json:"title"`
			} `json:"releases"`
		} `json:"recordings"`
	}
	path := fmt.Sprintf("/recording?query=%s&limit=%d&fmt=json", url.QueryEscape(query), searchLimit)
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	candidates := make([]models.Track, 0, len(out.Recordings))
	for _, r := range out.Recordings {
		if r.ID == "" || r.Title == "" {
			continue
		}
		var artistNames []string
		for _, a := range r.ArtistCredit {
			artistNames = append(artistNames, a.Name)
		}
		var album string
		if len(r.Releases) > 0 {
			album = r.Releases[0].Title
		}
		candidates = append(candidates, models.Track{
			MBID: r.ID, Title: r.Title, ArtistName: strings.Join(artistNames, " "), AlbumName: album,
		})
	}
	return candidates, nil
}

// LookupISRC returns an ISRC MusicBrainz has on file for the recording mbid,
// or "" if it has none. Backfills a track's ISRC after it was matched by text
// search (SearchRecording) rather than by ISRC in the first place. When a
// recording carries several ISRCs (reissues/alternate registrations), the
// first one MusicBrainz returns is used.
func (c *Client) LookupISRC(ctx context.Context, mbid string) (string, error) {
	if mbid == "" {
		return "", nil
	}
	var out struct {
		ISRCs []string `json:"isrcs"`
	}
	if err := c.get(ctx, "/recording/"+url.PathEscape(mbid)+"?inc=isrcs&fmt=json", &out); err != nil {
		return "", err
	}
	if len(out.ISRCs) == 0 {
		return "", nil
	}
	return out.ISRCs[0], nil
}

// escapeLucene escapes the two characters that would break out of a quoted
// Lucene phrase (MusicBrainz search syntax); track/artist names don't
// otherwise need Lucene's full special-character escaping inside quotes.
func escapeLucene(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}
