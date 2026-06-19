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
// fixed). All requests carry any configured headers (e.g. an auth token).
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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/providers"
)

// Provider is a generic HTTP-backed provider. Its config uses the shared
// providers.Config schema: Header are sent on every request, Params are appended
// to every request's query string. DownloadRetries: only the pre-stream phase
// (request + status) is retried; once audio bytes flow they are never replayed.
type Provider struct {
	name     string
	endpoint string
	cfg      providers.Config
	http     *http.Client
}

// Endpoint paths on the remote service — fixed by the protocol. The remote must
// implement these exact paths under its base URL. /capabilities is mandatory and
// validated when the provider is created (see Verify); the rest are called at
// use time.
const (
	capabilitiesPath  = "/capabilities"
	searchPath        = "/search"
	resolvePath       = "/resolve"
	downloadPath      = "/download"
	searchArtistsPath = "/artists"
	artistAlbumsPath  = "/artist/albums"
	artistTracksPath  = "/artist/tracks"
	albumTracksPath   = "/album/tracks"
	artistImagePath   = "/artist/image"
)

// slugRe constrains the name a remote declares for itself.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

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

	cfg, err := providers.ParseConfig(configJSON)
	if err != nil {
		return nil, fmt.Errorf("httpprovider: %w", err)
	}
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
		http:     providers.NewHTTPClient(timeout),
	}, nil
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

// Capabilities implements providers.CapabilityProvider: it fetches the remote's
// mandatory /capabilities endpoint and returns its advertised contract. Used by
// the admin to generate the config skeleton and display the live version, and by
// Verify. The request carries the configured header/params (authed discovery).
func (p *Provider) Capabilities(ctx context.Context) (providers.Capabilities, error) {
	req, err := p.newRequest(ctx, capabilitiesPath, url.Values{})
	if err != nil {
		return providers.Capabilities{}, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return providers.Capabilities{}, fmt.Errorf("%s: capabilities request failed (a /capabilities endpoint is required): %w", p.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return providers.Capabilities{}, fmt.Errorf("%s: /capabilities returned status %d (a /capabilities endpoint is required)", p.name, resp.StatusCode)
	}
	var caps providers.Capabilities
	if err := json.NewDecoder(io.LimitReader(resp.Body, providers.MaxMetadataBytes)).Decode(&caps); err != nil {
		return providers.Capabilities{}, fmt.Errorf("%s: decode capabilities: %w", p.name, err)
	}
	return caps, nil
}

// Verify implements providers.Verifier. It fetches /capabilities and checks the
// protocol version matches, the declared name is a slug, and every field marked
// required is present in the config in its declared location (header or params).
// A failure rejects activation of the provider.
func (p *Provider) Verify(ctx context.Context) error {
	caps, err := p.Capabilities(ctx)
	if err != nil {
		return err
	}
	if caps.Version != providers.ProtocolVersion {
		return fmt.Errorf("%s: remote protocol version %d unsupported (expected %d)", p.name, caps.Version, providers.ProtocolVersion)
	}
	if !slugRe.MatchString(caps.Name) {
		return fmt.Errorf("%s: capabilities name %q is not a valid slug", p.name, caps.Name)
	}
	var missing []string
	for key, f := range caps.Config {
		if !f.Required {
			continue
		}
		var got string
		switch f.Where {
		case "headers":
			got = p.cfg.Headers[key]
		case "params":
			got = p.cfg.Param(key, "")
		default:
			return fmt.Errorf("%s: config field %q has invalid location %q (want \"headers\" or \"params\")", p.name, key, f.Where)
		}
		if strings.TrimSpace(got) == "" {
			missing = append(missing, fmt.Sprintf("%s (%s)", key, f.Where))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s: missing required config field(s): %s", p.name, strings.Join(missing, ", "))
	}
	return nil
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
	// Merge static config params, without overriding the protocol params
	// (q/limit/id) the caller already set.
	for k, v := range p.cfg.Params {
		if q.Get(k) == "" {
			q.Set(k, v)
		}
	}
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

// statusError builds the error for a non-2xx response, appending the response
// body — it carries the upstream's real reason and is shown in full in the admin
// provider logs, so "deezer: search status 502" becomes
// "deezer: search status 502: <body>".
// ponytail: body capped at 8 KiB — the whole body for any real error response
// while bounding a pathological one; raise the cap if a provider needs more.
func (p *Provider) statusError(op string, resp *http.Response) error {
	const maxBody = 8 << 10
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	body := strings.TrimSpace(string(raw))
	if body == "" {
		return fmt.Errorf("%s: %s status %d", p.name, op, resp.StatusCode)
	}
	return fmt.Errorf("%s: %s status %d: %s", p.name, op, resp.StatusCode, body)
}

// Search implements providers.Provider.
func (p *Provider) Search(ctx context.Context, query string, limit int) ([]providers.Result, error) {
	if limit <= 0 {
		limit = 20
	}
	q := url.Values{"q": {query}, "limit": {strconv.Itoa(limit)}}
	req, err := p.newRequest(ctx, searchPath, q)
	if err != nil {
		return nil, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, p.statusError("search", resp)
	}
	var body struct {
		Results []track `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, providers.MaxMetadataBytes)).Decode(&body); err != nil {
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
	req, err := p.newRequest(ctx, resolvePath, url.Values{"id": {providerTrackID}})
	if err != nil {
		return providers.Result{}, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return providers.Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return providers.Result{}, p.statusError("resolve", resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, providers.MaxMetadataBytes))
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
		_, cErr := io.Copy(w, io.LimitReader(resp.Body, providers.MaxDownloadBytes))
		resp.Body.Close()
		return cErr
	}
	return lastErr
}

// openDownload issues the download request and returns the response only when it
// carries a streamable 2xx body. Any error returned here is from the pre-stream
// phase (nothing written to the caller yet), so it is safe to retry.
func (p *Provider) openDownload(ctx context.Context, providerTrackID string) (*http.Response, error) {
	req, err := p.newRequest(ctx, downloadPath, url.Values{"id": {providerTrackID}})
	if err != nil {
		return nil, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		err := p.statusError("download", resp)
		resp.Body.Close()
		return nil, err
	}
	return resp, nil
}
