package providers

import "context"

// ProtocolVersion is the capabilities protocol HTTP providers must implement.
// A remote's /capabilities must advertise this exact version.
const ProtocolVersion = 1

// Capabilities is the contract a provider advertises: HTTP providers at their
// mandatory /capabilities endpoint, built-ins in code. It states the protocol
// version, the slug name (which becomes the provider's name in immerle), the
// config fields it understands (keyed by field name), and the optional features
// it implements. An admin can switch any declared feature off per provider.
type Capabilities struct {
	Version int                    `json:"version"`
	Name    string                 `json:"name"`
	Config  map[string]ConfigField `json:"config"`
	// TrackSearch declares the /search endpoint (Provider.Search). Defaults to
	// true for an HTTP provider that omits it (protocol v1 compatibility); set
	// it to false for a provider that only serves playlists.
	TrackSearch bool `json:"trackSearch"`
	// Artists declares the artist/album browsing endpoints (ArtistSearcher,
	// ArtistAlbumLister, ArtistBrowser, AlbumBrowser, ArtistImageSearcher).
	// Defaults to true for an HTTP provider that omits it.
	Artists bool `json:"artists"`
	// Playlists declares the optional /playlists endpoint (PlaylistBrowser).
	Playlists bool `json:"playlists,omitempty"`
	// PlaylistSearch declares the optional /playlists/search endpoint
	// (PlaylistSearcher). When set, playlist search is delegated to the remote
	// instead of filtering /playlists by name.
	PlaylistSearch bool `json:"playlistSearch,omitempty"`
}

// Capability keys, as used in the protocol JSON and the admin toggles.
const (
	CapTrackSearch    = "trackSearch"
	CapArtists        = "artists"
	CapPlaylists      = "playlists"
	CapPlaylistSearch = "playlistSearch"
)

// CapabilityKeys lists every toggleable capability, in display order.
var CapabilityKeys = []string{CapTrackSearch, CapArtists, CapPlaylists, CapPlaylistSearch}

// DefaultCapabilities is what a provider that declares nothing is assumed to
// support: track search and artist browsing (missing endpoints answer 404 and
// degrade to empty results), no playlists.
func DefaultCapabilities() Capabilities {
	return Capabilities{Version: ProtocolVersion, TrackSearch: true, Artists: true}
}

func (c *Capabilities) flag(key string) *bool {
	switch key {
	case CapTrackSearch:
		return &c.TrackSearch
	case CapArtists:
		return &c.Artists
	case CapPlaylists:
		return &c.Playlists
	case CapPlaylistSearch:
		return &c.PlaylistSearch
	}
	return nil
}

// Has reports whether the capability key is declared.
func (c Capabilities) Has(key string) bool {
	f := c.flag(key)
	return f != nil && *f
}

// Without returns c with the given capability keys switched off.
func (c Capabilities) Without(keys []string) Capabilities {
	for _, k := range keys {
		if f := c.flag(k); f != nil {
			*f = false
		}
	}
	return c
}

// Keys returns the declared capability keys, in CapabilityKeys order.
func (c Capabilities) Keys() []string {
	var out []string
	for _, k := range CapabilityKeys {
		if c.Has(k) {
			out = append(out, k)
		}
	}
	return out
}

// ConfigField declares one config value the remote accepts: its value type
// (free-form, e.g. "string"), where it travels (a request header or query
// param), and whether it must be supplied for the provider to work.
type ConfigField struct {
	Type     string `json:"type"`
	Where    string `json:"where"` // "headers" | "params"
	Required bool   `json:"required"`
}

// CapabilityProvider is implemented by every provider: HTTP ones fetch their
// remote's /capabilities, built-ins answer in code. Used to gate which calls
// are made to a provider, and by the admin (config skeleton, live version,
// capability toggles).
type CapabilityProvider interface {
	Capabilities(ctx context.Context) (Capabilities, error)
}
