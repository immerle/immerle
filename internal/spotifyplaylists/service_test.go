package spotifyplaylists

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/spotifyweb"
	"github.com/immerle/immerle/internal/testutil"
)

// A featured artist or a different Unicode form on Spotify must not hide a
// track the library files under its main artist: the entry is keyed by the
// main artist and linked to the local track at sync time.
func TestTrackRefUsesMainArtistAndLinksLocalTrack(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	artistID, err := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Vacra", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	albumID, err := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Fille à Papa", ArtistID: artistID, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	trackID, err := store.Catalog.UpsertTrack(ctx, models.Track{
		// NFD, as macOS-written tags often are; Spotify sends NFC below.
		ID: uuid.NewString(), Title: "Fille a\u0300 Papa", AlbumID: albumID, ArtistID: artistID,
		Path: "/music/fille-a-papa.mp3", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	s := New(nil, store.Playlists, store.Catalog, nil, func() bool { return true }, nil, testutil.NewLogger())
	ref := s.trackRef(ctx, spotifyweb.Track{
		Title: "Fille \u00e0 Papa", Artist: "Vacra, La Rvfleuze", Artists: []string{"Vacra", "La Rvfleuze"}, Album: "Fille à Papa",
	})
	if ref.Artist != "Vacra" || ref.TrackID != trackID {
		t.Fatalf("ref = %+v, want artist Vacra linked to local track %s", ref, trackID)
	}

	ref = s.trackRef(ctx, spotifyweb.Track{Title: "Inconnu", Artist: "Vacra", Artists: []string{"Vacra"}})
	if ref.TrackID != "" {
		t.Fatalf("unknown track linked to %q, want lazy resolution", ref.TrackID)
	}
}
