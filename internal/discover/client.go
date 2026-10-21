// Package discover talks to immerle-discover (discover.immerle.com): a shared
// music catalog fed by charts and popular playlists, scrobble tracking per
// listener, and track/playlist suggestions. The instance registers itself on
// first use (no account) and keeps the returned credentials in the runtime
// settings. Listeners are opaque per-user references, never names or emails.
package discover

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/immerle/immerle/internal/models"
)

// maxResponseBytes caps an in-memory response body (a playlist with its
// tracks is the largest reply).
const maxResponseBytes = 4 << 20

// ErrDisabled means the admin switched discover (or the feature) off.
var ErrDisabled = errors.New("discover: disabled")

// Artist is a discover catalog artist. IDs are prefixed: "imlid:",
// "spotifyid:", "mbid:" (same for Track and Playlist).
type Artist struct {
	IDs  []string `json:"ids,omitempty"`
	Name string   `json:"name"`
}

// Track is a discover catalog track.
type Track struct {
	IDs     []string `json:"ids,omitempty"`
	Artist  string   `json:"artist"`
	Artists []Artist `json:"artists,omitempty"`
	Title   string   `json:"title"`
}

// MBID returns the track's MusicBrainz id, "" when discover has none.
func (t Track) MBID() string {
	for _, id := range t.IDs {
		if v, ok := strings.CutPrefix(id, "mbid:"); ok {
			return v
		}
	}
	return ""
}

// Playlist is an ingested playlist: a chart (Kind "chart") or one made by
// people (Kind "playlist").
type Playlist struct {
	IDs        []string        `json:"ids"`
	Kind       string          `json:"kind,omitempty"`
	Name       string          `json:"name"`
	Owner      string          `json:"owner,omitempty"`
	TrackCount int             `json:"trackCount"`
	Tracks     []PlaylistEntry `json:"tracks,omitempty"`
}

// PlaylistEntry is one track of a Playlist.
type PlaylistEntry struct {
	Track Track `json:"track"`
}

// ID is the playlist's discover id (the imlid, listed first).
func (p Playlist) ID() string {
	if len(p.IDs) == 0 {
		return ""
	}
	return p.IDs[0]
}

// Play is one completed play sent with Scrobble.
type Play struct {
	IDs      []string `json:"ids,omitempty"`
	Artist   string   `json:"artist"`
	Title    string   `json:"title"`
	Album    string   `json:"album,omitempty"`
	PlayedAt string   `json:"playedAt"`
}

// Credentials are the instance id and private key issued at registration.
type Credentials struct {
	InstanceID string
	PrivateKey string
}

// Client is the discover API client. settings is read live (hot-reloadable
// toggles); save persists credentials obtained at registration; secret keys
// the opaque listener references.
type Client struct {
	baseURL  string
	http     *http.Client
	settings func() models.DiscoverRuntime
	save     func(Credentials) error
	secret   func() string

	// ponytail: one lock around registration, it happens once per instance.
	mu sync.Mutex
}

// New builds a client. hc is optional (nil uses a 15s timeout).
func New(baseURL string, hc *http.Client, settings func() models.DiscoverRuntime, save func(Credentials) error, secret func() string) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: hc, settings: settings, save: save, secret: secret}
}

// ScrobbleEnabled reports whether plays are sent to discover.
func (c *Client) ScrobbleEnabled() bool {
	s := c.settings()
	return s.Enabled && s.Scrobble
}

// MadeForYouEnabled reports whether the "Mix discover" lists are published.
func (c *Client) MadeForYouEnabled() bool {
	s := c.settings()
	return s.Enabled && s.MadeForYou
}

// CuratedPlaylistsEnabled reports whether curated playlists are proposed.
func (c *Client) CuratedPlaylistsEnabled() bool {
	s := c.settings()
	return s.Enabled && s.CuratedPlaylists
}

// SimilarTracksEnabled reports whether discover backs similar tracks.
func (c *Client) SimilarTracksEnabled() bool {
	s := c.settings()
	return s.Enabled && s.SimilarTracks
}

// Listener is the opaque reference discover knows a user by: an HMAC of the
// user id under the instance secret, so it can't be reversed or linked across
// instances.
func (c *Client) Listener(userID string) string {
	m := hmac.New(sha256.New, []byte(c.secret()))
	m.Write([]byte(userID))
	return "u_" + hex.EncodeToString(m.Sum(nil))[:24]
}

// Scrobble records up to 50 completed plays for a user.
func (c *Client) Scrobble(ctx context.Context, userID string, plays []Play) error {
	body := map[string]any{"listener": c.Listener(userID), "scrobbles": plays}
	return c.do(ctx, http.MethodPost, "/api/v1/scrobbles", body, nil)
}

// ForgetListener deletes a user's listener and plays from discover. It runs
// even while discover is switched off (the data was sent before); an instance
// that never registered has nothing to delete.
func (c *Client) ForgetListener(ctx context.Context, userID string) error {
	s := c.settings()
	if s.InstanceID == "" || s.PrivateKey == "" {
		return nil
	}
	creds := Credentials{InstanceID: s.InstanceID, PrivateKey: s.PrivateKey}
	status, err := c.send(ctx, http.MethodDelete, "/api/v1/listeners/"+c.Listener(userID), creds, nil, nil)
	if status == http.StatusNotFound || status == http.StatusUnauthorized {
		return nil // already gone, or the whole instance was deleted
	}
	return err
}

// ListenerTracks returns tracks suggested from the user's recent plays.
func (c *Client) ListenerTracks(ctx context.Context, userID string, limit int) ([]Track, error) {
	var out struct {
		Tracks []Track `json:"tracks"`
	}
	path := fmt.Sprintf("/api/v1/listeners/%s/suggestions/tracks?limit=%d", c.Listener(userID), limit)
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Tracks, err
}

// ListenerPlaylists returns the editorial playlists overlapping the most with
// the user's recent plays.
func (c *Client) ListenerPlaylists(ctx context.Context, userID string, limit int) ([]Playlist, error) {
	var out struct {
		Playlists []Playlist `json:"playlists"`
	}
	path := fmt.Sprintf("/api/v1/listeners/%s/suggestions/playlists?limit=%d", c.Listener(userID), limit)
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Playlists, err
}

// Playlist returns one playlist with up to limit tracks.
func (c *Client) Playlist(ctx context.Context, id string, limit int) (Playlist, error) {
	var out struct {
		Playlist Playlist `json:"playlist"`
	}
	path := fmt.Sprintf("/api/v1/playlists/%s?limit=%d", url.PathEscape(id), limit)
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Playlist, err
}

// SimilarTracks returns tracks similar to the seeds (1-25).
func (c *Client) SimilarTracks(ctx context.Context, seeds []Track, limit int) ([]Track, error) {
	var out struct {
		Tracks []Track `json:"tracks"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/suggestions/tracks", map[string]any{"seeds": seeds, "limit": limit}, &out)
	return out.Tracks, err
}

// do sends an authenticated request, registering the instance first when it
// has no credentials yet. A 401 (key revoked, instance deleted) drops the
// credentials so the next call registers again.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	if !c.settings().Enabled {
		return ErrDisabled
	}
	creds, err := c.credentials(ctx)
	if err != nil {
		return err
	}
	status, err := c.send(ctx, method, path, creds, body, out)
	if status == http.StatusUnauthorized {
		if serr := c.save(Credentials{}); serr != nil {
			return serr
		}
	}
	return err
}

func (c *Client) credentials(ctx context.Context) (Credentials, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.settings(); s.InstanceID != "" && s.PrivateKey != "" {
		return Credentials{InstanceID: s.InstanceID, PrivateKey: s.PrivateKey}, nil
	}
	var out struct {
		Instance struct {
			ID string `json:"id"`
		} `json:"instance"`
		PrivateKey string `json:"privateKey"`
	}
	if _, err := c.send(ctx, http.MethodPost, "/api/v1/instances", Credentials{}, map[string]string{"name": "immerle"}, &out); err != nil {
		return Credentials{}, fmt.Errorf("discover: register: %w", err)
	}
	if out.Instance.ID == "" || out.PrivateKey == "" {
		return Credentials{}, errors.New("discover: register: empty credentials")
	}
	creds := Credentials{InstanceID: out.Instance.ID, PrivateKey: out.PrivateKey}
	return creds, c.save(creds)
}

func (c *Client) send(ctx context.Context, method, path string, creds Credentials, body, out any) (int, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if creds.InstanceID != "" {
		req.Header.Set("X-Instance-ID", creds.InstanceID)
		req.Header.Set("Authorization", "Bearer "+creds.PrivateKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("discover: %s %s: status %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("discover: decode %s: %w", path, err)
	}
	return resp.StatusCode, nil
}
