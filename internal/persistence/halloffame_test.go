package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestHallOfFame(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "owner", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	trackIDs := make([]string, 3)
	for i := range trackIDs {
		id, err := store.Catalog.UpsertTrack(ctx, models.Track{
			ID: uuid.NewString(), Title: "t", AlbumID: albumID, ArtistID: artistID,
			Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		trackIDs[i] = id
	}

	// GetOrCreate is idempotent: a second call returns the same row.
	h1, err := store.HallOfFame.GetOrCreate(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := store.HallOfFame.GetOrCreate(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if h1.ID != h2.ID {
		t.Fatalf("GetOrCreate not idempotent: %q != %q", h1.ID, h2.ID)
	}

	// Setting the order and duplicating a track id: the duplicate collapses to
	// its first occurrence, so only 3 entries survive.
	if err := store.HallOfFame.ReplaceEntries(ctx, h1.ID, []string{trackIDs[0], trackIDs[1], trackIDs[0], trackIDs[2]}); err != nil {
		t.Fatal(err)
	}
	entries, err := store.HallOfFame.Entries(ctx, h1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	for i, want := range []string{trackIDs[0], trackIDs[1], trackIDs[2]} {
		if entries[i].Track.ID != want {
			t.Fatalf("entry %d = %q, want %q", i, entries[i].Track.ID, want)
		}
	}

	// A note survives a reorder (ReplaceEntries deletes and reinserts rows).
	if err := store.HallOfFame.SetNote(ctx, h1.ID, trackIDs[0], "listened to this in college"); err != nil {
		t.Fatal(err)
	}
	if err := store.HallOfFame.ReplaceEntries(ctx, h1.ID, []string{trackIDs[2], trackIDs[1], trackIDs[0]}); err != nil {
		t.Fatal(err)
	}
	entries, err = store.HallOfFame.Entries(ctx, h1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[2].Track.ID != trackIDs[0] || entries[2].Comment != "listened to this in college" {
		t.Fatalf("note did not survive reorder: %+v", entries)
	}

	// Clearing the note (empty comment) removes it.
	if err := store.HallOfFame.SetNote(ctx, h1.ID, trackIDs[0], ""); err != nil {
		t.Fatal(err)
	}
	entries, err = store.HallOfFame.Entries(ctx, h1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if entries[2].Comment != "" {
		t.Fatalf("expected cleared comment, got %q", entries[2].Comment)
	}
}
