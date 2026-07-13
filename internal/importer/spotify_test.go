package importer

import (
	"testing"

	"github.com/immerle/immerle/internal/spotifyweb"
)

func TestSpotifyBuildsWithoutHub(t *testing.T) {
	src, err := newSpotify(SourceDeps{})
	if err != nil {
		t.Fatalf("spotify should build without a hub: %v", err)
	}
	if src.Name() != "spotify" {
		t.Fatalf("Name() = %q, want spotify", src.Name())
	}
}

func TestToImporterPlaylist(t *testing.T) {
	pl := toImporterPlaylist(spotifyweb.Playlist{Tracks: []spotifyweb.Track{
		{Title: "Da Funk", Artist: "Daft Punk", Album: "Homework", URI: "spotify:track:x"},
	}})
	if len(pl.Tracks) != 1 || pl.Tracks[0].Title != "Da Funk" || pl.Tracks[0].Artist != "Daft Punk" || pl.Tracks[0].Album != "Homework" {
		t.Fatalf("playlist not mapped: %+v", pl)
	}
}
