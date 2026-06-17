// Package httpprovider implements a generic, content-neutral on-demand provider
// that delegates to an external HTTP service. It is configured entirely at
// runtime with a name, a base endpoint and an opaque JSON config — the core
// neither knows nor cares what the remote service does. This is the seam for
// plugging in any out-of-process catalog/downloader you operate and have the
// rights to use.
//
// # Remote protocol
//
// The external service must expose three endpoints under the base URL (paths are
// configurable). All requests carry any configured headers (e.g. an auth token).
//
//	GET  {endpoint}{searchPath}?q={query}&limit={n}
//	     → 200 application/json: {"results": [ <Track>, ... ]}
//	GET  {endpoint}{resolvePath}?id={providerTrackId}
//	     → 200 application/json: <Track>            (or {"result": <Track>})
//	GET  {endpoint}{downloadPath}?id={providerTrackId}
//	     → 200 with the raw audio bytes as the response body
//
// where <Track> is:
//
//	{
//	  "providerTrackId": "abc", "title": "...", "artist": "...", "album": "...",
//	  "albumArtist": "...", "trackNo": 1, "discNo": 1, "year": 2020,
//	  "duration": 210, "genre": "...", "mbid": "...",
//	  "providerArtistId": "...", "coverImageUrl": "...", "artistImageUrl": "...",
//	  "suffix": "mp3"
//	}
package httpprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gossignol/gossignol/internal/providers"
)

// Config is the JSON config payload supplied per provider. Every field is
// optional; only Headers typically needs setting (to authenticate to the
// service). Paths default to /search, /resolve and /download.
type Config struct {
	// Headers are sent on every request (e.g. {"Authorization": "Bearer ..."}).
	Headers map[string]string `json:"headers,omitempty"`
	// SearchPath, ResolvePath, DownloadPath override the default endpoint paths.
	SearchPath   string `json:"searchPath,omitempty"`
	ResolvePath  string `json:"resolvePath,omitempty"`
	DownloadPath string `json:"downloadPath,omitempty"`
	// Artist/album browsing paths (optional capabilities). A service that does
	// not implement one should return 404 — the provider then degrades quietly
	// (the artist page just won't be enriched from it). Defaults: /artists,
	// /artist/albums, /artist/tracks, /album/tracks.
	SearchArtistsPath string `json:"searchArtistsPath,omitempty"`
	ArtistAlbumsPath  string `json:"artistAlbumsPath,omitempty"`
	ArtistTracksPath  string `json:"artistTracksPath,omitempty"`
	AlbumTracksPath   string `json:"albumTracksPath,omitempty"`
	ArtistImagePath   string `json:"artistImagePath,omitempty"`
	// Quality is a free-form label reported as the provider's MaxQuality.
	Quality string `json:"quality,omitempty"`
	// TimeoutSeconds bounds each HTTP call (default 60).
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
	// DownloadRetries is the number of attempts for a download before giving up
	// (default 3). Only the pre-stream phase (request + status) is retried; once
	// audio bytes start flowing to the caller they are never replayed.
	DownloadRetries int `json:"downloadRetries,omitempty"`
}

// Provider is a generic HTTP-backed provider.
type Provider struct {
	name     string
	endpoint string
	cfg      Config
	http     *http.Client
}

// New builds an HTTP provider. endpoint must be an absolute http(s) URL;
// configJSON is the raw config payload ("" or "{}" for defaults).
func New(name, endpoint, configJSON string) (*Provider, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("httpprovider: name is required")
	}
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("httpprovider: endpoint must be an absolute http(s) URL")
	}

	var cfg Config
	if s := strings.TrimSpace(configJSON); s != "" {
		if err := json.Unmarshal([]byte(s), &cfg); err != nil {
			return nil, fmt.Errorf("httpprovider: invalid config JSON: %w", err)
		}
	}
	cfg.SearchPath = pathOr(cfg.SearchPath, "/search")
	cfg.ResolvePath = pathOr(cfg.ResolvePath, "/resolve")
	cfg.DownloadPath = pathOr(cfg.DownloadPath, "/download")
	cfg.SearchArtistsPath = pathOr(cfg.SearchArtistsPath, "/artists")
	cfg.ArtistAlbumsPath = pathOr(cfg.ArtistAlbumsPath, "/artist/albums")
	cfg.ArtistTracksPath = pathOr(cfg.ArtistTracksPath, "/artist/tracks")
	cfg.AlbumTracksPath = pathOr(cfg.AlbumTracksPath, "/album/tracks")
	cfg.ArtistImagePath = pathOr(cfg.ArtistImagePath, "/artist/image")
	timeout := 60 * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	if cfg.DownloadRetries <= 0 {
		cfg.DownloadRetries = 3
	}

	return &Provider{
		name:     name,
		endpoint: endpoint,
		cfg:      cfg,
		http:     &http.Client{Timeout: timeout},
	}, nil
}

func pathOr(p, def string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return def
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// Name implements providers.Provider.
func (p *Provider) Name() string { return p.name }

// MaxQuality implements providers.Provider.
func (p *Provider) MaxQuality() string {
	if p.cfg.Quality != "" {
		return p.cfg.Quality
	}
	return "remote"
}

// track is the wire shape of a result returned by the remote service.
type track struct {
	ProviderTrackID  string `json:"providerTrackId"`
	Title            string `json:"title"`
	Artist           string `json:"artist"`
	Album            string `json:"album"`
	AlbumArtist      string `json:"albumArtist"`
	TrackNo          int    `json:"trackNo"`
	DiscNo           int    `json:"discNo"`
	Year             int    `json:"year"`
	Duration         int    `json:"duration"`
	Genre            string `json:"genre"`
	MBID             string `json:"mbid"`
	ProviderArtistID string `json:"providerArtistId"`
	CoverImageURL    string `json:"coverImageUrl"`
	ArtistImageURL   string `json:"artistImageUrl"`
	Suffix           string `json:"suffix"`
}

func (t track) toResult() providers.Result {
	suffix := t.Suffix
	if suffix == "" {
		suffix = "mp3"
	}
	return providers.Result{
		ProviderTrackID:  t.ProviderTrackID,
		Title:            t.Title,
		Artist:           t.Artist,
		Album:            t.Album,
		AlbumArtist:      t.AlbumArtist,
		TrackNo:          t.TrackNo,
		DiscNo:           t.DiscNo,
		Year:             t.Year,
		Duration:         t.Duration,
		Genre:            t.Genre,
		MBID:             t.MBID,
		ProviderArtistID: t.ProviderArtistID,
		CoverImageURL:    t.CoverImageURL,
		ArtistImageURL:   t.ArtistImageURL,
		Suffix:           suffix,
	}
}

func (p *Provider) newRequest(ctx context.Context, path string, q url.Values) (*http.Request, error) {
	u := p.endpoint + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range p.cfg.Headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// Search implements providers.Provider.
func (p *Provider) Search(ctx context.Context, query string, limit int) ([]providers.Result, error) {
	if limit <= 0 {
		limit = 20
	}
	q := url.Values{"q": {query}, "limit": {strconv.Itoa(limit)}}
	req, err := p.newRequest(ctx, p.cfg.SearchPath, q)
	if err != nil {
		return nil, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: search status %d", p.name, resp.StatusCode)
	}
	var body struct {
		Results []track `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("%s: decode search: %w", p.name, err)
	}
	out := make([]providers.Result, 0, len(body.Results))
	for _, t := range body.Results {
		if t.ProviderTrackID == "" {
			continue
		}
		out = append(out, t.toResult())
	}
	return out, nil
}

// Resolve implements providers.Provider. It accepts either a bare track object
// or {"result": <Track>}.
func (p *Provider) Resolve(ctx context.Context, providerTrackID string) (providers.Result, error) {
	req, err := p.newRequest(ctx, p.cfg.ResolvePath, url.Values{"id": {providerTrackID}})
	if err != nil {
		return providers.Result{}, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return providers.Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return providers.Result{}, fmt.Errorf("%s: resolve status %d", p.name, resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return providers.Result{}, err
	}
	var wrapped struct {
		Result *track `json:"result"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Result != nil {
		return wrapped.Result.toResult(), nil
	}
	var t track
	if err := json.Unmarshal(raw, &t); err != nil {
		return providers.Result{}, fmt.Errorf("%s: decode resolve: %w", p.name, err)
	}
	if t.ProviderTrackID == "" {
		return providers.Result{}, fmt.Errorf("%s: track %q not found", p.name, providerTrackID)
	}
	return t.toResult(), nil
}

// downloadRetryBackoff is the base delay between download attempts (grows
// linearly per attempt).
const downloadRetryBackoff = 300 * time.Millisecond

// Download implements providers.Provider by streaming the service's audio bytes.
// Transient failures of the *open phase* (network error, or a non-2xx status —
// e.g. the remote service momentarily failing to mint a token) are retried up to
// DownloadRetries times. Once a 2xx body is acquired and bytes begin flowing to
// w they are never replayed: a mid-stream error fails the call (the partially
// written output can't be rewound).
func (p *Provider) Download(ctx context.Context, providerTrackID string, w io.Writer) error {
	attempts := p.cfg.DownloadRetries
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := p.openDownload(ctx, providerTrackID)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if attempt < attempts {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(attempt) * downloadRetryBackoff):
				}
				continue
			}
			break
		}
		// 2xx stream acquired — bytes now flow to w and cannot be replayed.
		_, cErr := io.Copy(w, resp.Body)
		resp.Body.Close()
		return cErr
	}
	return lastErr
}

// openDownload issues the download request and returns the response only when it
// carries a streamable 2xx body. Any error returned here is from the pre-stream
// phase (nothing written to the caller yet), so it is safe to retry.
func (p *Provider) openDownload(ctx context.Context, providerTrackID string) (*http.Response, error) {
	req, err := p.newRequest(ctx, p.cfg.DownloadPath, url.Values{"id": {providerTrackID}})
	if err != nil {
		return nil, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: download status %d", p.name, resp.StatusCode)
	}
	return resp, nil
}
