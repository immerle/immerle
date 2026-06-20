// Package podcastsearch holds the built-in podcast directory adapters. Each one
// searches an external directory and returns RSS feed URLs to subscribe to (the
// audio itself stays on the publisher's feed — these are discovery only).
//
// Adapters are compiled in; the admin enables one and supplies the per-source
// config it declares via ConfigFields (some need an API key, some need nothing).
package podcastsearch

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ConfigField describes one credential/setting an adapter needs, so the admin UI
// can render the right input (and mark secrets as password fields).
type ConfigField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Secret   bool   `json:"secret"`
}

// Result is one directory hit: the feed URL is what feeds createPodcastChannel.
type Result struct {
	Title   string `json:"title"`
	Author  string `json:"author"`
	FeedURL string `json:"feedUrl"`
	Image   string `json:"image"`
	Source  string `json:"source"`
}

// Provider is a built-in podcast directory adapter.
type Provider interface {
	Name() string                // stable slug, e.g. "itunes"
	DisplayName() string         // human label, e.g. "Apple Podcasts"
	ConfigFields() []ConfigField // what the admin must supply (empty = none)
	Search(ctx context.Context, query string, cfg map[string]string) ([]Result, error)
}

// Builtins returns every compiled-in adapter, sharing one HTTP client.
func Builtins(client *http.Client) []Provider {
	return []Provider{
		&itunes{client: client},
		&podcastIndex{client: client},
		&listenNotes{client: client},
		&fyyd{client: client},
		&gpodder{client: client},
	}
}

// --- Apple Podcasts (iTunes Search API): no auth, the biggest directory ---

type itunes struct{ client *http.Client }

func (i *itunes) Name() string                { return "itunes" }
func (i *itunes) DisplayName() string         { return "Apple Podcasts" }
func (i *itunes) ConfigFields() []ConfigField { return nil }

func (i *itunes) Search(ctx context.Context, query string, _ map[string]string) ([]Result, error) {
	q := url.Values{"media": {"podcast"}, "term": {query}, "limit": {"25"}}
	var body struct {
		Results []struct {
			CollectionName string `json:"collectionName"`
			ArtistName     string `json:"artistName"`
			FeedURL        string `json:"feedUrl"`
			Artwork        string `json:"artworkUrl600"`
		} `json:"results"`
	}
	if err := getJSON(ctx, i.client, "https://itunes.apple.com/search?"+q.Encode(), nil, &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Results))
	for _, r := range body.Results {
		if r.FeedURL == "" {
			continue
		}
		out = append(out, Result{Title: r.CollectionName, Author: r.ArtistName, FeedURL: r.FeedURL, Image: r.Artwork, Source: "itunes"})
	}
	return out, nil
}

// --- Podcast Index: the biggest open directory; needs key + secret (signed) ---

type podcastIndex struct{ client *http.Client }

func (p *podcastIndex) Name() string        { return "podcastindex" }
func (p *podcastIndex) DisplayName() string { return "Podcast Index" }
func (p *podcastIndex) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "apiKey", Label: "API Key", Required: true, Secret: false},
		{Key: "apiSecret", Label: "API Secret", Required: true, Secret: true},
	}
}

func (p *podcastIndex) Search(ctx context.Context, query string, cfg map[string]string) ([]Result, error) {
	key, secret := cfg["apiKey"], cfg["apiSecret"]
	if key == "" || secret == "" {
		return nil, fmt.Errorf("podcastindex: apiKey and apiSecret are required")
	}
	// Auth is a per-request signature: sha1(key + secret + unix-seconds). A static
	// header map can't express this — that's why it's a compiled adapter.
	authDate := strconv.FormatInt(time.Now().Unix(), 10)
	sum := sha1.Sum([]byte(key + secret + authDate))
	headers := map[string]string{
		"X-Auth-Key":    key,
		"X-Auth-Date":   authDate,
		"Authorization": hex.EncodeToString(sum[:]),
	}
	q := url.Values{"q": {query}, "max": {"25"}}
	var body struct {
		Feeds []struct {
			Title  string `json:"title"`
			Author string `json:"author"`
			URL    string `json:"url"`
			Image  string `json:"image"`
		} `json:"feeds"`
	}
	if err := getJSON(ctx, p.client, "https://api.podcastindex.org/api/1.0/search/byterm?"+q.Encode(), headers, &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Feeds))
	for _, f := range body.Feeds {
		if f.URL == "" {
			continue
		}
		out = append(out, Result{Title: f.Title, Author: f.Author, FeedURL: f.URL, Image: f.Image, Source: "podcastindex"})
	}
	return out, nil
}

// --- Listen Notes: large curated directory; needs an API key (static header) ---

type listenNotes struct{ client *http.Client }

func (l *listenNotes) Name() string        { return "listennotes" }
func (l *listenNotes) DisplayName() string { return "Listen Notes" }
func (l *listenNotes) ConfigFields() []ConfigField {
	return []ConfigField{{Key: "apiKey", Label: "API Key", Required: true, Secret: true}}
}

func (l *listenNotes) Search(ctx context.Context, query string, cfg map[string]string) ([]Result, error) {
	key := cfg["apiKey"]
	if key == "" {
		return nil, fmt.Errorf("listennotes: apiKey is required")
	}
	q := url.Values{"q": {query}, "type": {"podcast"}}
	var body struct {
		Results []struct {
			Title     string `json:"title_original"`
			Publisher string `json:"publisher_original"`
			RSS       string `json:"rss"`
			Image     string `json:"image"`
		} `json:"results"`
	}
	url := "https://listen-api.listennotes.com/api/v2/search?" + q.Encode()
	if err := getJSON(ctx, l.client, url, map[string]string{"X-ListenAPI-Key": key}, &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Results))
	for _, r := range body.Results {
		if r.RSS == "" {
			continue
		}
		out = append(out, Result{Title: r.Title, Author: r.Publisher, FeedURL: r.RSS, Image: r.Image, Source: "listennotes"})
	}
	return out, nil
}

// --- fyyd: large open podcast search engine; no auth ---

type fyyd struct{ client *http.Client }

func (f *fyyd) Name() string                { return "fyyd" }
func (f *fyyd) DisplayName() string         { return "fyyd" }
func (f *fyyd) ConfigFields() []ConfigField { return nil }

func (f *fyyd) Search(ctx context.Context, query string, _ map[string]string) ([]Result, error) {
	// ponytail: search by title. fyyd also has a broader `term`; swap if title-only
	// proves too narrow.
	q := url.Values{"title": {query}, "count": {"25"}}
	var body struct {
		Data []struct {
			Title  string `json:"title"`
			Author string `json:"author"`
			XMLURL string `json:"xmlURL"`
			ImgURL string `json:"imgURL"`
		} `json:"data"`
	}
	if err := getJSON(ctx, f.client, "https://api.fyyd.de/0.2/search/podcast?"+q.Encode(), nil, &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Data))
	for _, d := range body.Data {
		if d.XMLURL == "" {
			continue
		}
		out = append(out, Result{Title: d.Title, Author: d.Author, FeedURL: d.XMLURL, Image: d.ImgURL, Source: "fyyd"})
	}
	return out, nil
}

// --- gpodder.net: the open community directory; no auth ---

type gpodder struct{ client *http.Client }

func (g *gpodder) Name() string                { return "gpodder" }
func (g *gpodder) DisplayName() string         { return "gpodder.net" }
func (g *gpodder) ConfigFields() []ConfigField { return nil }

func (g *gpodder) Search(ctx context.Context, query string, _ map[string]string) ([]Result, error) {
	q := url.Values{"q": {query}}
	// gpodder returns a top-level JSON array of podcasts.
	var body []struct {
		Title   string `json:"title"`
		Author  string `json:"author"`
		URL     string `json:"url"`
		LogoURL string `json:"logo_url"`
	}
	if err := getJSON(ctx, g.client, "https://gpodder.net/search.json?"+q.Encode(), nil, &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body))
	for _, d := range body {
		if d.URL == "" {
			continue
		}
		out = append(out, Result{Title: d.Title, Author: d.Author, FeedURL: d.URL, Image: d.LogoURL, Source: "gpodder"})
	}
	return out, nil
}

// getJSON does a GET with optional headers and decodes the JSON response.
func getJSON(ctx context.Context, client *http.Client, url string, headers map[string]string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "immerle")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("search %s: %s", url, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(dst)
}
