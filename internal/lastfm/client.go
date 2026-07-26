// Package lastfm submits plays to Last.fm (last.fm) on behalf of a user, via
// its "desktop auth" handshake: the server holds an app-level API key +
// shared secret (registered on Last.fm's developer site, admin-configured),
// each user obtains their own permanent session key by visiting an
// api_key-scoped auth URL and approving it once.
package lastfm

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// defaultBaseURL is Last.fm's REST API root.
const defaultBaseURL = "https://ws.audioscrobbler.com/2.0/"

// authURLBase is where a user is sent to approve a token (desktop-auth flow).
const authURLBase = "https://www.last.fm/api/auth/"

// maxResponseBytes caps an in-memory response body, same reasoning as the
// ListenBrainz client -- guards against a hostile/broken reply.
const maxResponseBytes = 1 << 20

// Sentinel errors, mapped from Last.fm's numeric error codes.
var (
	// ErrInvalidSession means Last.fm rejected the session key (code 9).
	ErrInvalidSession = errors.New("lastfm: invalid session")
	// ErrPending means the token hasn't been approved by the user yet (code
	// 14) -- the caller should let them retry after visiting the auth URL.
	ErrPending = errors.New("lastfm: token not authorized yet")
	// ErrRateLimited means Last.fm answered with a rate-limit error (code 29).
	ErrRateLimited = errors.New("lastfm: rate limited")
)

// Client talks to the Last.fm API. baseURL is overridable (tests point it at
// an httptest.Server) instead of a package-level default.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a client against the real Last.fm API. hc is optional
// (nil uses a client with a sane default timeout).
func NewClient(hc *http.Client) *Client {
	return newClient(defaultBaseURL, hc)
}

func newClient(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{baseURL: baseURL, http: hc}
}

// sign computes a Last.fm API signature: md5 of every param's key+value,
// sorted by key, concatenated with no delimiter, followed by the shared
// secret. format/callback are never signed -- callers must not pass them in.
func sign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(params[k])
	}
	b.WriteString(secret)
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// errorEnvelope is the shape of a Last.fm error response.
type errorEnvelope struct {
	Error   int    `json:"error"`
	Message string `json:"message"`
}

// do signs and sends an API call, returning the raw JSON body on success.
// httpMethod is GET for read methods, POST for methods with side effects
// (auth.getToken/getSession are GET; track.scrobble is POST, per Last.fm's
// own docs).
func (c *Client) do(ctx context.Context, httpMethod, apiMethod, apiKey, apiSecret string, params map[string]string) ([]byte, error) {
	full := make(map[string]string, len(params)+2)
	for k, v := range params {
		full[k] = v
	}
	full["method"] = apiMethod
	full["api_key"] = apiKey
	sig := sign(full, apiSecret)

	values := url.Values{}
	for k, v := range full {
		values.Set(k, v)
	}
	values.Set("api_sig", sig)
	values.Set("format", "json")

	var req *http.Request
	var err error
	if httpMethod == http.MethodPost {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, strings.NewReader(values.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"?"+values.Encode(), nil)
	}
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}

	var errEnv errorEnvelope
	if json.Unmarshal(body, &errEnv) == nil && errEnv.Error != 0 {
		switch errEnv.Error {
		case 9:
			return nil, ErrInvalidSession
		case 14:
			return nil, ErrPending
		case 29:
			return nil, ErrRateLimited
		default:
			return nil, fmt.Errorf("lastfm: %s: error %d: %s", apiMethod, errEnv.Error, errEnv.Message)
		}
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("lastfm: %s: unexpected status %d", apiMethod, resp.StatusCode)
	}
	return body, nil
}

// GetToken requests a fresh auth token (auth.getToken), the first step of the
// desktop-auth handshake.
func (c *Client) GetToken(ctx context.Context, apiKey, apiSecret string) (string, error) {
	body, err := c.do(ctx, http.MethodGet, "auth.getToken", apiKey, apiSecret, nil)
	if err != nil {
		return "", err
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("lastfm: decode auth.getToken: %w", err)
	}
	return out.Token, nil
}

// AuthURL builds the page a user visits to approve a token.
func (c *Client) AuthURL(apiKey, token string) string {
	v := url.Values{"api_key": {apiKey}, "token": {token}}
	return authURLBase + "?" + v.Encode()
}

// GetSession exchanges an approved token for a permanent session key
// (auth.getSession), the last step of the handshake. Returns ErrPending if
// the user hasn't approved the token yet.
func (c *Client) GetSession(ctx context.Context, apiKey, apiSecret, token string) (sessionKey, username string, err error) {
	body, err := c.do(ctx, http.MethodGet, "auth.getSession", apiKey, apiSecret, map[string]string{"token": token})
	if err != nil {
		return "", "", err
	}
	var out struct {
		Session struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		} `json:"session"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", "", fmt.Errorf("lastfm: decode auth.getSession: %w", err)
	}
	return out.Session.Key, out.Session.Name, nil
}

// Listen is one play, ready to scrobble.
type Listen struct {
	ListenedAt time.Time
	Artist     string
	Track      string
	Release    string
	DurationMs int
}

// Scrobble submits a single completed play (track.scrobble).
func (c *Client) Scrobble(ctx context.Context, apiKey, apiSecret, sessionKey string, l Listen) error {
	params := map[string]string{
		"sk":        sessionKey,
		"artist":    l.Artist,
		"track":     l.Track,
		"timestamp": strconv.FormatInt(l.ListenedAt.Unix(), 10),
	}
	if l.Release != "" {
		params["album"] = l.Release
	}
	if l.DurationMs > 0 {
		params["duration"] = strconv.Itoa(l.DurationMs / 1000)
	}
	_, err := c.do(ctx, http.MethodPost, "track.scrobble", apiKey, apiSecret, params)
	return err
}

// SimilarTrack is one track.getSimilar result.
type SimilarTrack struct {
	Artist string
	Title  string
}

// GetSimilarTracks returns tracks similar to (artist, track), per Last.fm's
// listening/tag similarity graph (track.getSimilar). This is a public method
// -- it needs the app-level API key/secret but no user session key, unlike
// Scrobble/GetSession.
func (c *Client) GetSimilarTracks(ctx context.Context, apiKey, apiSecret, artist, track string, limit int) ([]SimilarTrack, error) {
	params := map[string]string{"artist": artist, "track": track, "autocorrect": "1"}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}
	body, err := c.do(ctx, http.MethodGet, "track.getSimilar", apiKey, apiSecret, params)
	if err != nil {
		return nil, err
	}
	var out struct {
		SimilarTracks struct {
			Track []struct {
				Name   string `json:"name"`
				Artist struct {
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"track"`
		} `json:"similartracks"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("lastfm: decode track.getSimilar: %w", err)
	}
	similar := make([]SimilarTrack, 0, len(out.SimilarTracks.Track))
	for _, t := range out.SimilarTracks.Track {
		similar = append(similar, SimilarTrack{Artist: t.Artist.Name, Title: t.Name})
	}
	return similar, nil
}
