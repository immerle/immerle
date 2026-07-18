// Package bandcamp talks to Bandcamp's unofficial fan/collection API using a
// pasted "identity" session cookie — Bandcamp has no official OAuth for this.
// Endpoints and the pagedata-blob scrape are reverse-engineered; verified
// against the community easlice/bandcamp-downloader tool and the
// michaelherger/Bandcamp-API reverse-engineered OpenAPI spec. May break if
// Bandcamp changes its markup or API shape.
package bandcamp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"time"
)

// baseURL is a var, not a const, so tests can point it at an httptest server.
var baseURL = "https://bandcamp.com"

// ErrInvalidCookie means the identity cookie is missing, expired, or not
// logged in.
var ErrInvalidCookie = errors.New("bandcamp: cookie is invalid or expired")

// ErrPagedataNotFound means the download page didn't have the expected
// embedded data — the most fragile part of this integration, since it depends
// on Bandcamp's page markup staying stable.
var ErrPagedataNotFound = errors.New("bandcamp: pagedata blob not found (page layout may have changed)")

// ErrNoDownloadAvailable means the download page had no usable format.
var ErrNoDownloadAvailable = errors.New("bandcamp: no download available for this item")

// ErrDownloadTooLarge means the download exceeded the caller's byte limit.
var ErrDownloadTooLarge = errors.New("bandcamp: download exceeds size limit")

// formatPriority is the fixed, best-first format choice order.
var formatPriority = []string{"flac", "mp3-320", "aac-hi", "alac", "mp3-v0", "vorbis", "wav", "aiff-lossless"}

// Client talks to bandcamp.com on behalf of a user's pasted session cookie.
// The cookie is passed per-call rather than stored on the Client, since one
// server-side Client is shared across every connected user.
type Client struct{ http *http.Client }

// NewClient builds a Client.
func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) newRequest(ctx context.Context, method, url, identityCookie string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", "identity="+identityCookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; immerle)")
	return req, nil
}

type collectionSummaryResponse struct {
	FanID json.Number `json:"fan_id"`
}

// FanID validates the cookie and returns the fan id it resolves to.
func (c *Client) FanID(ctx context.Context, identityCookie string) (string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, baseURL+"/api/fan/2/collection_summary", identityCookie, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", ErrInvalidCookie
	}
	var body collectionSummaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.FanID == "" {
		return "", ErrInvalidCookie
	}
	return body.FanID.String(), nil
}

// CollectionItem is one purchased item (album or track). SaleItemType+SaleItemID
// is Bandcamp's own unique key for it.
type CollectionItem struct {
	SaleItemType  string
	SaleItemID    string
	ItemType      string // "album" | "track"
	ArtistName    string
	ItemTitle     string
	AlbumTitle    string
	Purchased     time.Time
	ArtURL        string
	RedownloadURL string // "" if Bandcamp didn't return one for this item
}

// CollectionPage is one page of a fan's purchase collection.
type CollectionPage struct {
	Items         []CollectionItem
	MoreAvailable bool
	LastToken     string
}

type collectionItemsRequest struct {
	FanID          string `json:"fan_id"`
	OlderThanToken string `json:"older_than_token"`
	Count          int    `json:"count"`
}

type collectionItemsResponse struct {
	MoreAvailable bool   `json:"more_available"`
	LastToken     string `json:"last_token"`
	Items         []struct {
		SaleItemType string      `json:"sale_item_type"`
		SaleItemID   json.Number `json:"sale_item_id"`
		ItemType     string      `json:"item_type"`
		BandName     string      `json:"band_name"`
		ItemTitle    string      `json:"item_title"`
		AlbumTitle   string      `json:"album_title"`
		Purchased    string      `json:"purchased"`
		ItemArtURL   string      `json:"item_art_url"`
	} `json:"items"`
	RedownloadURLs map[string]string `json:"redownload_urls"`
}

// bandcampDateLayout is the format Bandcamp uses for the "purchased" field,
// e.g. "01 Jan 2021 10:00:00 GMT".
const bandcampDateLayout = "02 Jan 2006 15:04:05 GMT"

// Collection fetches one page of a fan's purchase collection
// (POST /api/fancollection/1/collection_items). olderThanToken == "" fetches
// the first page; pass CollectionPage.LastToken to fetch the next one.
func (c *Client) Collection(ctx context.Context, identityCookie, fanID, olderThanToken string, count int) (CollectionPage, error) {
	reqBody, err := json.Marshal(collectionItemsRequest{FanID: fanID, OlderThanToken: olderThanToken, Count: count})
	if err != nil {
		return CollectionPage{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, baseURL+"/api/fancollection/1/collection_items", identityCookie, bytes.NewReader(reqBody))
	if err != nil {
		return CollectionPage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return CollectionPage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return CollectionPage{}, fmt.Errorf("bandcamp: unexpected status %d listing collection", resp.StatusCode)
	}
	var body collectionItemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return CollectionPage{}, err
	}
	page := CollectionPage{MoreAvailable: body.MoreAvailable, LastToken: body.LastToken}
	for _, it := range body.Items {
		key := it.SaleItemType + it.SaleItemID.String()
		item := CollectionItem{
			SaleItemType:  it.SaleItemType,
			SaleItemID:    it.SaleItemID.String(),
			ItemType:      it.ItemType,
			ArtistName:    it.BandName,
			ItemTitle:     it.ItemTitle,
			AlbumTitle:    it.AlbumTitle,
			ArtURL:        it.ItemArtURL,
			RedownloadURL: body.RedownloadURLs[key],
		}
		if t, err := time.Parse(bandcampDateLayout, it.Purchased); err == nil {
			item.Purchased = t
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

// pagedataRe matches the one <div id="pagedata" data-blob="..."> on a
// Bandcamp download page.
var pagedataRe = regexp.MustCompile(`<div id="pagedata" data-blob="([^"]*)"`)

// DownloadInfo is the resolved, directly-downloadable link for a purchased
// item in the best format Bandcamp has available.
type DownloadInfo struct {
	URL    string
	Format string
	SizeMB float64
}

type pagedataBlob struct {
	DownloadItems []struct {
		Downloads map[string]struct {
			URL    string  `json:"url"`
			SizeMB float64 `json:"size_mb"`
		} `json:"downloads"`
	} `json:"download_items"`
}

// ResolveDownload fetches a redownload_url page and picks the best available
// format per formatPriority. No further polling is needed — the URL it
// returns is directly downloadable.
func (c *Client) ResolveDownload(ctx context.Context, identityCookie, redownloadURL string) (DownloadInfo, error) {
	req, err := c.newRequest(ctx, http.MethodGet, redownloadURL, identityCookie, nil)
	if err != nil {
		return DownloadInfo{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return DownloadInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return DownloadInfo{}, fmt.Errorf("bandcamp: unexpected status %d fetching download page", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return DownloadInfo{}, err
	}
	m := pagedataRe.FindSubmatch(raw)
	if m == nil {
		return DownloadInfo{}, ErrPagedataNotFound
	}
	unescaped := html.UnescapeString(string(m[1]))
	var blob pagedataBlob
	if err := json.Unmarshal([]byte(unescaped), &blob); err != nil {
		return DownloadInfo{}, fmt.Errorf("bandcamp: parsing pagedata blob: %w", err)
	}
	if len(blob.DownloadItems) == 0 {
		return DownloadInfo{}, ErrNoDownloadAvailable
	}
	downloads := blob.DownloadItems[0].Downloads
	for _, format := range formatPriority {
		if d, ok := downloads[format]; ok {
			return DownloadInfo{URL: d.URL, Format: format, SizeMB: d.SizeMB}, nil
		}
	}
	return DownloadInfo{}, ErrNoDownloadAvailable
}

// Download streams url (cookie attached) to w, aborting once more than
// maxBytes have been written.
func (c *Client) Download(ctx context.Context, identityCookie, url string, w io.Writer, maxBytes int64) error {
	req, err := c.newRequest(ctx, http.MethodGet, url, identityCookie, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("bandcamp: unexpected status %d downloading file", resp.StatusCode)
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return err
	}
	if n > maxBytes {
		return ErrDownloadTooLarge
	}
	return nil
}
