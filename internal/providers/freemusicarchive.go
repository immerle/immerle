package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// FreeMusicArchiveProvider serves Creative-Commons-licensed audio from the Free
// Music Archive (freemusicarchive.org). FMA shut down its public API, so this
// provider scrapes the public search page and streams tracks via FMA's own
// /stream/ redirect (which points at the authorized file CDN). Requests carry a
// browser User-Agent because the site fronts its HTML with a CDN that rejects
// non-browser clients.
//
// ponytail: HTML-scraping is inherently brittle — title/artist/handle come from
// the embedded data-track-info JSON (stable), album/genre/duration are
// best-effort from sibling markup (empty if the layout changes).
type FreeMusicArchiveProvider struct {
	baseURL string
	http    *http.Client
}

// browserUA is sent on every request to look like a real browser.
const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// NewFreeMusicArchiveProvider builds an FMA provider.
func NewFreeMusicArchiveProvider(baseURL string) *FreeMusicArchiveProvider {
	if baseURL == "" {
		baseURL = "https://freemusicarchive.org"
	}
	return &FreeMusicArchiveProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    NewHTTPClient(60 * time.Second),
	}
}

func init() {
	RegisterFactory("free-music-archive", func(settings map[string]string) (Provider, error) {
		return NewFreeMusicArchiveProvider(setting(settings, "base_url", "")), nil
	})
}

// Name implements Provider.
func (p *FreeMusicArchiveProvider) Name() string { return "free-music-archive" }

// MaxQuality implements Provider. FMA streams are mp3.
func (p *FreeMusicArchiveProvider) MaxQuality() string { return "mp3" }

// fmaTrack is the metadata packed into a provider track id, so Resolve needs no
// network round-trip (FMA exposes no per-track metadata endpoint by handle).
type fmaTrack struct {
	Handle   string `json:"h"`
	Title    string `json:"t"`
	Artist   string `json:"a"`
	Album    string `json:"al,omitempty"`
	Genre    string `json:"g,omitempty"`
	Duration int    `json:"d,omitempty"`
}

func (t fmaTrack) toResult() Result {
	return Result{
		ProviderTrackID: t.encode(),
		Title:           t.Title,
		Artist:          t.Artist,
		Album:           t.Album,
		AlbumArtist:     t.Artist,
		Genre:           t.Genre,
		Duration:        t.Duration,
		Suffix:          "mp3",
	}
}

func (t fmaTrack) encode() string {
	b, _ := json.Marshal(t)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeFMAID(id string) (fmaTrack, error) {
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return fmaTrack{}, fmt.Errorf("free-music-archive: invalid track id %q", id)
	}
	var t fmaTrack
	if err := json.Unmarshal(b, &t); err != nil || t.Handle == "" {
		return fmaTrack{}, fmt.Errorf("free-music-archive: invalid track id %q", id)
	}
	return t, nil
}

// data-track-info holds a JSON blob per track; siblings carry album/genre/duration.
var (
	reTrackInfo = regexp.MustCompile(`(?s)data-track-info='(\{.*?\})'`)
	reAlbum     = regexp.MustCompile(`(?s)ptxt-album.*?<a[^>]*>\s*(.*?)\s*</a>`)
	reGenre     = regexp.MustCompile(`(?s)ptxt-genre.*?<a[^>]*>\s*(.*?)\s*</a>`)
	reDuration  = regexp.MustCompile(`\b(\d{1,2}:\d{2}(?::\d{2})?)\b`)
)

type fmaInfo struct {
	Handle     string `json:"handle"`
	Title      string `json:"title"`
	ArtistName string `json:"artistName"`
}

// Search implements Provider by scraping the public quicksearch page.
func (p *FreeMusicArchiveProvider) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 20
	}
	u := p.baseURL + "/search?quicksearch=" + url.QueryEscape(query)
	doc, err := p.getHTML(ctx, u)
	if err != nil {
		return nil, err
	}

	locs := reTrackInfo.FindAllStringSubmatchIndex(doc, -1)
	out := make([]Result, 0, len(locs))
	for i, loc := range locs {
		// Block spans from this track's data-track-info to the next track's.
		end := len(doc)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		block := doc[loc[0]:end]

		var info fmaInfo
		if err := json.Unmarshal([]byte(doc[loc[2]:loc[3]]), &info); err != nil || info.Handle == "" {
			continue
		}
		out = append(out, fmaTrack{
			Handle:   info.Handle,
			Title:    info.Title,
			Artist:   info.ArtistName,
			Album:    firstGroup(reAlbum, block),
			Genre:    firstGroup(reGenre, block),
			Duration: parseDuration(firstGroup(reDuration, block)),
		}.toResult())
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Resolve implements Provider — fully from the packed id, no network call.
func (p *FreeMusicArchiveProvider) Resolve(ctx context.Context, providerTrackID string) (Result, error) {
	t, err := decodeFMAID(providerTrackID)
	if err != nil {
		return Result{}, err
	}
	return t.toResult(), nil
}

// Download implements Provider via FMA's /stream/ endpoint, which 302-redirects
// to the authorized file CDN (the http client follows it).
func (p *FreeMusicArchiveProvider) Download(ctx context.Context, providerTrackID string, w io.Writer) error {
	t, err := decodeFMAID(providerTrackID)
	if err != nil {
		return err
	}
	u := p.baseURL + "/track/" + url.PathEscape(t.Handle) + "/stream/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", browserUA)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("free-music-archive: download status %d", resp.StatusCode)
	}
	_, err = io.Copy(w, io.LimitReader(resp.Body, MaxDownloadBytes))
	return err
}

func (p *FreeMusicArchiveProvider) getHTML(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html")
	resp, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("free-music-archive: status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxMetadataBytes))
	return string(b), err
}

func firstGroup(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(m[1]))
}

// parseDuration converts "mm:ss" or "hh:mm:ss" to seconds.
func parseDuration(s string) int {
	if s == "" {
		return 0
	}
	secs := 0
	for _, part := range strings.Split(s, ":") {
		n, _ := strconv.Atoi(part)
		secs = secs*60 + n
	}
	return secs
}
