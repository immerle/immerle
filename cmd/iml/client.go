package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ErrUnauthorized marks a 401: the caller's Bearer token is missing, invalid
// or expired -- distinct from a network/server error, since only this one
// means "log in again".
var ErrUnauthorized = errors.New("unauthorized")

// Client is a minimal client for the native immerle API (/api/v1), auth'd via
// a Bearer device-session JWT obtained from Login.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{}}
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

// do sends a request under /api/v1 and decodes the JSON response into out
// (nil to discard the body). A non-2xx status is turned into an error using
// the server's {"error":{code,message}} envelope.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody *strings.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = strings.NewReader(string(data))
	} else {
		reqBody = strings.NewReader("")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1"+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode >= 300 {
		var env errorEnvelope
		_ = json.NewDecoder(resp.Body).Decode(&env)
		if env.Error.Message != "" {
			return fmt.Errorf("%s: %s", path, env.Error.Message)
		}
		return fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Login exchanges credentials for a device-session JWT, used as the Bearer
// token on every later request.
func (c *Client) Login(ctx context.Context, username, password string) error {
	var out struct {
		Token string `json:"token"`
	}
	if err := c.do(ctx, http.MethodPost, "/auth/sessions", map[string]string{
		"username": username,
		"password": password,
		"device":   "iml",
	}, &out); err != nil {
		return err
	}
	c.token = out.Token
	return nil
}

// Me checks that the current Bearer token is still valid (returns
// ErrUnauthorized if not).
func (c *Client) Me(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/me", nil, nil)
}

// Song, Album and Playlist mirror just the fields this client needs from the
// server's songView/albumView/playlistView (unexported types, so we can't
// import them -- these are plain JSON shapes instead).
type Song struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
}

type Album struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Artist string `json:"artist"`
}

type Playlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SongCount int    `json:"songCount"`
}

type searchHit struct {
	Type     string    `json:"type"`
	Song     *Song     `json:"song"`
	Album    *Album    `json:"album"`
	Playlist *Playlist `json:"playlist"`
}

// Search scopes server-side to one result type ("song", "album" or
// "playlist"): GET /search?type=... already zeroes the other types' counts.
func (c *Client) Search(ctx context.Context, query, scope string) ([]Song, []Album, []Playlist, error) {
	var out struct {
		Results []searchHit `json:"results"`
	}
	path := "/search?" + url.Values{"q": {query}, "type": {scope}}.Encode()
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, nil, nil, err
	}
	var songs []Song
	var albums []Album
	var playlists []Playlist
	for _, h := range out.Results {
		switch {
		case h.Song != nil:
			songs = append(songs, *h.Song)
		case h.Album != nil:
			albums = append(albums, *h.Album)
		case h.Playlist != nil:
			playlists = append(playlists, *h.Playlist)
		}
	}
	return songs, albums, playlists, nil
}

func (c *Client) AlbumTracks(ctx context.Context, id string) ([]Song, error) {
	var out struct {
		Tracks []Song `json:"tracks"`
	}
	if err := c.do(ctx, http.MethodGet, "/albums/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return out.Tracks, nil
}

func (c *Client) PlaylistTracks(ctx context.Context, id string) ([]Song, error) {
	var out struct {
		Tracks []Song `json:"tracks"`
	}
	if err := c.do(ctx, http.MethodGet, "/playlists/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return out.Tracks, nil
}

// StreamURL mints a short-lived signed stream URL for a track: no
// Authorization header needed, so it can be handed straight to a plain
// http.Get (the player doesn't carry the Bearer token).
//
// The id must be percent-encoded in the request path (matching what the web
// UI's fetch client does): an on-demand/remote track id is
// "remote:<provider>:<base64 provider id>", and that base64 payload can
// contain "/" or "+" -- passed raw, those get read back as extra path
// segments (or otherwise break routing) and the server 404s.
func (c *Client) StreamURL(ctx context.Context, id string) (string, error) {
	var out struct {
		Stream string `json:"stream"`
	}
	if err := c.do(ctx, http.MethodGet, "/songs/"+url.PathEscape(id)+"/stream-url", nil, &out); err != nil {
		return "", err
	}
	return c.baseURL + out.Stream, nil
}
