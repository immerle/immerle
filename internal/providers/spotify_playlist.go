package providers

import (
	"context"
	"errors"
	"io"
)

// errSpotifyNoTracks is returned for track operations: Spotify is never a
// download source.
var errSpotifyNoTracks = errors.New("spotify-playlist: playlists only, tracks resolve through the other providers")

// SpotifyProvider is the built-in switch for public Spotify playlists in
// search (see internal/spotifyplaylists). It contributes no tracks: a mirrored
// playlist's tracks resolve at play time through the local catalog and the
// other providers. Enabling or disabling it turns the feature on or off.
type SpotifyProvider struct{}

func init() {
	RegisterFactory("spotify-playlist", func(Config) (Provider, error) { return SpotifyProvider{}, nil })
}

// Name implements Provider.
func (SpotifyProvider) Name() string { return "spotify-playlist" }

// Capabilities implements CapabilityProvider: playlist search only. Switching
// playlistSearch off in the admin turns the feature off like disabling it.
func (SpotifyProvider) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{Version: ProtocolVersion, Name: "spotify-playlist", PlaylistSearch: true}, nil
}

// MaxQuality implements Provider.
func (SpotifyProvider) MaxQuality() string { return "playlists only" }

// Search implements Provider: no tracks.
func (SpotifyProvider) Search(context.Context, string, int) ([]Result, error) { return nil, nil }

// Resolve implements Provider.
func (SpotifyProvider) Resolve(context.Context, string) (Result, error) {
	return Result{}, errSpotifyNoTracks
}

// Download implements Provider.
func (SpotifyProvider) Download(context.Context, string, io.Writer) error {
	return errSpotifyNoTracks
}
