// Package providers defines the pluggable abstraction for external music
// catalogs used by the on-demand feature (S5), plus the shipped legal providers
// (Jamendo, Internet Archive).
package providers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// Response-size caps for the shipped HTTP-backed providers. A hostile or buggy
// remote must not be able to exhaust memory or disk with an unbounded body.
const (
	// MaxMetadataBytes caps an in-memory JSON/metadata response.
	MaxMetadataBytes = 8 << 20 // 8 MiB
	// MaxDownloadBytes caps a streamed audio download. Generous enough for any
	// real lossless track while bounding disk exhaustion.
	// ponytail: 1 GiB ceiling; raise if legitimately larger files appear.
	MaxDownloadBytes = 1 << 30 // 1 GiB
)

// NewHTTPClient builds the HTTP client used by the shipped providers: an overall
// request timeout plus a header timeout, so a remote that accepts the connection
// but never sends response headers cannot hang a request indefinitely.
func NewHTTPClient(timeout time.Duration) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Timeout: timeout, Transport: tr}
}

// ErrDownloadNotSupported is returned by a provider's Download when it only
// exposes search/metadata and intentionally does not retrieve audio (e.g. the
// Deezer provider, which uses the account ARL for metadata only — never to
// fetch or decrypt protected streams).
var ErrDownloadNotSupported = errors.New("download not supported by this provider (metadata only)")

// Result is a track found at a provider.
type Result struct {
	// ProviderTrackID uniquely identifies the track within the provider.
	ProviderTrackID string
	Title           string
	Artist          string
	Album           string
	AlbumArtist     string
	TrackNo         int
	DiscNo          int
	Year            int
	Duration        int
	Genre           string
	MBID            string
	// ProviderArtistID is the artist's id within the provider (optional; enables
	// browsing the artist's catalog).
	ProviderArtistID string
	// CoverImageURL and ArtistImageURL are absolute image URLs on the provider's
	// public image CDN (optional; used to serve remote cover art / avatars).
	CoverImageURL  string
	ArtistImageURL string
	// Suffix is the audio file extension the provider will deliver (e.g. "mp3").
	Suffix string
}

// ArtistBrowser is an optional capability: a provider that can return a given
// artist's tracks (used to browse a remote artist surfaced in search).
type ArtistBrowser interface {
	ArtistTracks(ctx context.Context, providerArtistID string, limit int) ([]Result, error)
}

// ArtistResult is an artist found at a provider.
type ArtistResult struct {
	ProviderArtistID string
	Name             string
	AlbumCount       int
	ImageURL         string
}

// ArtistSearcher is an optional capability: a provider that can search for
// artists directly (yielding accurate album counts and images), rather than
// having artists inferred from track results.
type ArtistSearcher interface {
	SearchArtists(ctx context.Context, query string, limit int) ([]ArtistResult, error)
}

// ArtistImageSearcher is an optional capability: resolve an artist's avatar URL
// by name. Avatars come from wherever artists do, so when a provider exposes
// this, local artist pages are enriched with its artwork. Returns "" if none.
type ArtistImageSearcher interface {
	ArtistImage(ctx context.Context, name string) (string, error)
}

// ProviderAlbum is an album in a provider's catalog.
type ProviderAlbum struct {
	ProviderAlbumID string
	Title           string
	Year            int
	CoverImageURL   string
}

// ArtistAlbumLister is an optional capability: list an artist's albums (its
// discography), used to enrich a local artist page with albums available
// remotely.
type ArtistAlbumLister interface {
	ArtistAlbums(ctx context.Context, providerArtistID string, limit int) ([]ProviderAlbum, error)
}

// AlbumBrowser is an optional capability: return an album's tracks by provider
// album id (used to browse a remote album).
type AlbumBrowser interface {
	AlbumTracks(ctx context.Context, providerAlbumID string, limit int) ([]Result, error)
}

// Provider is one external catalog (Jamendo, Internet Archive, or a runtime
// HTTP provider).
// Implementations must be safe for concurrent use.
type Provider interface {
	// Name is the stable provider identifier (used in download jobs and ids).
	Name() string
	// Search returns up to limit matching tracks.
	Search(ctx context.Context, query string, limit int) ([]Result, error)
	// Resolve returns the full result for a provider track id.
	Resolve(ctx context.Context, providerTrackID string) (Result, error)
	// Download writes the raw audio bytes of a provider track to w.
	Download(ctx context.Context, providerTrackID string, w io.Writer) error
	// MaxQuality describes the best quality the provider can deliver (free-form).
	MaxQuality() string
}
