package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestForgottenFavorites(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	user := models.User{ID: uuid.NewString(), Username: "u", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	newTrack := func(title string) string {
		id, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: title, AlbumID: albumID, ArtistID: artistID, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
		return id
	}
	neverPlayed := newTrack("Never Played But Liked")
	longAgo := newTrack("Liked, Played Long Ago")
	recent := newTrack("Liked, Played Recently")
	notStarred := newTrack("Not Starred")

	for _, id := range []string{neverPlayed, longAgo, recent, notStarred} {
		if id != notStarred {
			if err := store.Annotations.SetStarred(ctx, user.ID, models.ItemTrack, id, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := store.Annotations.IncrementPlay(ctx, user.ID, models.ItemTrack, longAgo, now.AddDate(0, 0, -120)); err != nil {
		t.Fatal(err)
	}
	if err := store.Annotations.IncrementPlay(ctx, user.ID, models.ItemTrack, recent, now.AddDate(0, 0, -5)); err != nil {
		t.Fatal(err)
	}

	got, err := store.Annotations.ForgottenFavorites(ctx, user.ID, models.ItemTrack, now.AddDate(0, 0, -90), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 forgotten favorites (never-played + long-ago), got %d: %+v", len(got), got)
	}
	ids := map[string]bool{got[0]: true, got[1]: true}
	if !ids[neverPlayed] || !ids[longAgo] {
		t.Fatalf("expected neverPlayed and longAgo, got %+v", got)
	}
	if ids[recent] || ids[notStarred] {
		t.Fatalf("recent and non-starred tracks must be excluded, got %+v", got)
	}
}
