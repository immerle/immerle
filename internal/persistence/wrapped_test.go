package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestWrappedAggregatesByYear(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	user := models.User{ID: uuid.NewString(), Username: "u", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Discovery", ArtistID: artistID, CreatedAt: now})
	hit, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "One More Time", AlbumID: albumID, ArtistID: artistID, Genre: "House", Duration: 320, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
	other, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "Aerodynamic", AlbumID: albumID, ArtistID: artistID, Genre: "House", Duration: 200, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})

	scrobble := func(trackID string, at time.Time) {
		if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: user.ID, TrackID: trackID, PlayedAt: at, Submitted: true}); err != nil {
			t.Fatal(err)
		}
	}
	// 3 plays of "hit" and 1 of "other" in 2024 (Jan + Mar); 1 play in 2023 (must be excluded).
	jan2024 := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	mar2024 := time.Date(2024, 3, 2, 9, 0, 0, 0, time.UTC)
	scrobble(hit, jan2024)
	scrobble(hit, jan2024)
	scrobble(hit, mar2024)
	scrobble(other, mar2024)
	scrobble(hit, time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))

	w, err := store.Wrapped.Wrapped(ctx, user.ID, 2024)
	if err != nil {
		t.Fatal(err)
	}
	if w.TotalPlays != 4 {
		t.Fatalf("TotalPlays = %d, want 4", w.TotalPlays)
	}
	if w.TotalSeconds != 320*3+200 {
		t.Fatalf("TotalSeconds = %d, want %d", w.TotalSeconds, 320*3+200)
	}
	if w.ByMonth[0] != 2 || w.ByMonth[2] != 2 {
		t.Fatalf("ByMonth Jan=%d Mar=%d, want 2 and 2", w.ByMonth[0], w.ByMonth[2])
	}
	if len(w.TopTracks) != 2 || w.TopTracks[0].ID != hit || w.TopTracks[0].Plays != 3 {
		t.Fatalf("TopTracks[0] = %+v, want hit with 3 plays", w.TopTracks)
	}
	if w.TopTracks[0].Artist != "Daft Punk" {
		t.Fatalf("artist = %q", w.TopTracks[0].Artist)
	}
	if len(w.TopArtists) != 1 || w.TopArtists[0].Plays != 4 {
		t.Fatalf("TopArtists = %+v, want one artist with 4 plays", w.TopArtists)
	}
	if len(w.TopGenres) != 1 || w.TopGenres[0].Name != "House" || w.TopGenres[0].Plays != 4 {
		t.Fatalf("TopGenres = %+v, want House with 4 plays", w.TopGenres)
	}
}
