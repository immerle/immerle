package importer

import (
	"context"

	"github.com/immerle/immerle/internal/spotifyweb"
)

func init() { RegisterFactory("spotify", newSpotify) }

// spotifySource imports a public Spotify playlist by calling Spotify's own
// web-player API directly (see internal/spotifyweb) — no Spotify credentials
// or hub needed, since the playlist is public.
type spotifySource struct {
	client *spotifyweb.Client
}

func newSpotify(SourceDeps) (Source, error) {
	return &spotifySource{client: spotifyweb.NewClient()}, nil
}

func (s *spotifySource) Name() string { return "spotify" }

func (s *spotifySource) FetchPlaylist(ctx context.Context, ref string) (Playlist, error) {
	pl, err := s.client.FetchPlaylist(ctx, ref)
	if err != nil {
		return Playlist{}, err
	}
	return toImporterPlaylist(pl), nil
}

func toImporterPlaylist(pl spotifyweb.Playlist) Playlist {
	tracks := make([]Track, len(pl.Tracks))
	for i, t := range pl.Tracks {
		tracks[i] = Track{Title: t.Title, Artist: t.Artist, Album: t.Album}
	}
	return Playlist{Tracks: tracks}
}
