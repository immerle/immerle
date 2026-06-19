package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

func TestTrackAdminListEditDelete(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Discovery", ArtistID: artistID, CreatedAt: now})

	local, err := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "One More Time", AlbumID: albumID, ArtistID: artistID,
		Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	// A remote (not-downloaded) track must be excluded from the admin listing.
	if _, err := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Aerodynamic", AlbumID: albumID, ArtistID: artistID,
		Remote: true, Provider: "jamendo", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	// List excludes the remote track.
	all, err := store.Catalog.ListAllTracks(ctx, persistence.TrackListOptions{Size: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != local {
		t.Fatalf("expected only the local track, got %d: %+v", len(all), all)
	}
	if n, _ := store.Catalog.CountTracks(ctx, ""); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}

	// Search matches by artist name (case-insensitive).
	hits, _ := store.Catalog.ListAllTracks(ctx, persistence.TrackListOptions{Query: "daft", Size: 50})
	if len(hits) != 1 {
		t.Fatalf("search hits = %d, want 1", len(hits))
	}
	if miss, _ := store.Catalog.ListAllTracks(ctx, persistence.TrackListOptions{Query: "nope", Size: 50}); len(miss) != 0 {
		t.Fatalf("expected no hits, got %d", len(miss))
	}

	// Edit metadata.
	if err := store.Catalog.UpdateTrackMetadata(ctx, local, "Harder Better", "House", 2001, 4, 1); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Catalog.GetTrack(ctx, local)
	if got.Title != "Harder Better" || got.Genre != "House" || got.Year != 2001 || got.TrackNo != 4 {
		t.Fatalf("metadata not updated: %+v", got)
	}
	if err := store.Catalog.UpdateTrackMetadata(ctx, "missing", "x", "", 0, 0, 0); err != persistence.ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing track, got %v", err)
	}

	// Set cover.
	if err := store.Catalog.SetTrackCover(ctx, local, "trk_"+local); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Catalog.GetTrack(ctx, local); got.CoverArt != "trk_"+local {
		t.Fatalf("cover not set: %q", got.CoverArt)
	}

	// An annotation for the track (no FK to tracks) must be removed by the cascade.
	user := models.User{ID: uuid.NewString(), Username: "u", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := store.Annotations.SetStarred(ctx, user.ID, models.ItemTrack, local, true); err != nil {
		t.Fatal(err)
	}

	if err := store.Catalog.DeleteTrackCascade(ctx, local); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Catalog.GetTrack(ctx, local); err != persistence.ErrNotFound {
		t.Fatalf("track should be gone, got %v", err)
	}
	if a, _ := store.Annotations.Get(ctx, user.ID, models.ItemTrack, local); a.Starred != nil {
		t.Fatal("annotation should have been cleaned up by cascade delete")
	}
}
