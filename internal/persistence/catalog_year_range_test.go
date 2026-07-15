package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestListTracksByYearRange(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	newTrack := func(title string, year int) string {
		id, err := store.Catalog.UpsertTrack(ctx, models.Track{
			ID: uuid.NewString(), Title: title, AlbumID: albumID, ArtistID: artistID,
			Year: year, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	nineties := newTrack("Nineties Hit", 1995)
	alsoNineties := newTrack("Another Nineties Hit", 1999)
	eighties := newTrack("Eighties Hit", 1985)
	noYear := newTrack("Unknown Year", 0)

	got, err := store.Catalog.ListTracksByYearRange(ctx, 1990, 2000, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tracks in [1990,2000), got %d: %+v", len(got), got)
	}
	ids := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !ids[nineties] || !ids[alsoNineties] {
		t.Fatalf("expected the two 90s tracks, got %+v", got)
	}
	if ids[eighties] || ids[noYear] {
		t.Fatalf("expected the 80s/unknown-year tracks excluded, got %+v", got)
	}
}

func TestTracksByIDs(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	id1, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "One", AlbumID: albumID, ArtistID: artistID, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
	id2, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "Two", AlbumID: albumID, ArtistID: artistID, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})

	got, err := store.Catalog.TracksByIDs(ctx, []string{id1, id2, "missing-id"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 resolved tracks, got %d: %+v", len(got), got)
	}
	if got[id1].Title != "One" || got[id2].Title != "Two" {
		t.Fatalf("unexpected tracks: %+v", got)
	}
}
