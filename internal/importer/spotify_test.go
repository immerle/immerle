package importer

import (
	"context"
	"errors"
	"testing"
)

// fakeHub is a stub HubFetcher capturing the requested source/ref.
type fakeHub struct {
	available         bool
	gotSource, gotRef string
	pl                Playlist
	err               error
}

func (f *fakeHub) Available() bool { return f.available }
func (f *fakeHub) FetchPlaylist(_ context.Context, source, ref string) (Playlist, error) {
	f.gotSource, f.gotRef = source, ref
	return f.pl, f.err
}

func TestSpotifyRequiresHub(t *testing.T) {
	if _, err := newSpotify(SourceDeps{Hub: nil}); err == nil {
		t.Fatal("spotify without a hub should be rejected")
	}
	if _, err := newSpotify(SourceDeps{Hub: &fakeHub{available: false}}); err == nil {
		t.Fatal("spotify with an unavailable hub should be rejected")
	}
	if _, err := newSpotify(SourceDeps{Hub: &fakeHub{available: true}}); err != nil {
		t.Fatalf("spotify with an available hub should build, got %v", err)
	}
}

func TestSpotifyDelegatesToHub(t *testing.T) {
	hub := &fakeHub{available: true, pl: Playlist{Name: "My Mix", Tracks: []Track{{Title: "Da Funk", Artist: "Daft Punk"}}}}
	src, err := newSpotify(SourceDeps{Hub: hub})
	if err != nil {
		t.Fatal(err)
	}
	pl, err := src.FetchPlaylist(context.Background(), "https://open.spotify.com/playlist/PL?si=x")
	if err != nil {
		t.Fatal(err)
	}
	if hub.gotSource != "spotify" || hub.gotRef != "https://open.spotify.com/playlist/PL?si=x" {
		t.Fatalf("hub called with source=%q ref=%q", hub.gotSource, hub.gotRef)
	}
	if pl.Name != "My Mix" || len(pl.Tracks) != 1 || pl.Tracks[0].Title != "Da Funk" {
		t.Fatalf("playlist not passed through: %+v", pl)
	}
}

func TestSpotifyPropagatesHubError(t *testing.T) {
	src, _ := newSpotify(SourceDeps{Hub: &fakeHub{available: true, err: errors.New("hub down")}})
	if _, err := src.FetchPlaylist(context.Background(), "PL"); err == nil {
		t.Fatal("expected the hub error to propagate")
	}
}
