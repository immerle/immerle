// Package spotifyweb fetches public Spotify playlists by replaying the
// GraphQL calls Spotify's own web player makes to api-partner.spotify.com,
// authenticated with the same anonymous access token the web player gets for
// a logged-out visitor. Minting that token requires answering a TOTP
// challenge whose secret Spotify rotates periodically; see currentSecret in
// token.go — it's fetched fresh from a community tracker on every token
// refresh, falling back to a hardcoded pair if that tracker is unreachable.
package spotifyweb

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/immerle/immerle/internal/providers"
)

const pageSize = 50

// userAgent identifies as a real browser: Spotify's endpoints reject the
// default Go HTTP client UA outright.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

// Track is a single playlist track as returned by Spotify's web player API.
type Track struct {
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
	URI      string
}

// Playlist is a public Spotify playlist's tracks. Spotify's pathfinder query
// used here (mirroring the web player's own scroll-pagination call) doesn't
// return the playlist's display name, only its content.
type Playlist struct {
	Tracks []Track
}

// Client fetches public Spotify playlists. The zero value is not usable; build
// one with NewClient. Safe for concurrent use.
type Client struct {
	http *http.Client

	mu     sync.Mutex
	cached accessToken
}

// NewClient builds a Client ready to fetch public Spotify playlists.
func NewClient() *Client {
	return &Client{http: providers.NewHTTPClient(30 * time.Second)}
}

// FetchPlaylist resolves a playlist reference (a share URL or a bare id) into
// its full track list, paginating as needed.
func (c *Client) FetchPlaylist(ctx context.Context, ref string) (Playlist, error) {
	id := playlistID(ref)
	if id == "" {
		return Playlist{}, fmt.Errorf("spotifyweb: could not find a playlist id in %q", ref)
	}

	tok, err := c.token(ctx)
	if err != nil {
		return Playlist{}, err
	}

	var tracks []Track
	for offset := 0; ; offset += pageSize {
		page, total, err := fetchTracksPage(ctx, c.http, tok, id, offset, pageSize)
		if err != nil {
			return Playlist{}, err
		}
		tracks = append(tracks, page...)
		if len(page) == 0 || offset+pageSize >= total {
			break
		}
	}
	return Playlist{Tracks: tracks}, nil
}

// token returns a still-valid access token, minting a fresh one if the cached
// one is missing or expired.
func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached.valid() {
		return c.cached.value, nil
	}
	tok, err := fetchAccessToken(ctx, c.http)
	if err != nil {
		return "", err
	}
	c.cached = tok
	return tok.value, nil
}

var playlistIDRe = regexp.MustCompile(`^[A-Za-z0-9]{22}$`)

// playlistID pulls the playlist id out of a share URL (any locale prefix, any
// query string) or accepts a bare id as-is.
func playlistID(ref string) string {
	ref = strings.TrimSpace(ref)
	if i := strings.Index(ref, "playlist/"); i >= 0 {
		id := ref[i+len("playlist/"):]
		if q := strings.IndexAny(id, "?/"); q >= 0 {
			id = id[:q]
		}
		if playlistIDRe.MatchString(id) {
			return id
		}
		return ""
	}
	if playlistIDRe.MatchString(ref) {
		return ref
	}
	return ""
}
