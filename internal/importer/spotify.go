package importer

import (
	"context"
	"fmt"
)

func init() { RegisterFactory("spotify", newSpotify) }

// spotifySource imports a public Spotify playlist through the immerle hub. The
// hub owns the Spotify credentials and exposes a "fetch public playlist"
// endpoint, so this source needs no client id/secret of its own — it just
// delegates, passing the user-supplied reference (id or URL) straight through.
type spotifySource struct {
	hub HubFetcher
}

func newSpotify(d SourceDeps) (Source, error) {
	if d.Hub == nil || !d.Hub.Available() {
		return nil, fmt.Errorf("spotify import requires a configured hub (set the federation hub URL and keys)")
	}
	return &spotifySource{hub: d.Hub}, nil
}

func (s *spotifySource) Name() string { return "spotify" }

func (s *spotifySource) FetchPlaylist(ctx context.Context, ref string) (Playlist, error) {
	return s.hub.FetchPlaylist(ctx, "spotify", ref)
}
