package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestSearchPublicMatchesNameAndRespectsVisibility(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	me := models.User{ID: uuid.NewString(), Username: "me", PasswordHash: "x", CreatedAt: now}
	other := models.User{ID: uuid.NewString(), Username: "other", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, me); err != nil {
		t.Fatal(err)
	}
	if err := store.Users.Create(ctx, other); err != nil {
		t.Fatal(err)
	}

	create := func(name, ownerID string, public bool) {
		t.Helper()
		p := models.Playlist{ID: uuid.NewString(), Name: name, OwnerID: ownerID, Public: public, CreatedAt: now, UpdatedAt: now}
		if err := store.Playlists.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	create("Road Trip Hits", other.ID, true)   // matches, public, someone else's → found
	create("Road Trip Demos", other.ID, false) // matches, private → excluded
	create("Chill Vibes", other.ID, true)      // public but doesn't match → excluded
	create("Road Trip Mix", me.ID, true)       // matches, public, but the caller's own → excluded (matches ListPublic)

	got, err := store.Playlists.SearchPublic(ctx, me.ID, "road trip", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Road Trip Hits" {
		t.Fatalf("SearchPublic = %+v, want only %q", got, "Road Trip Hits")
	}
}

func TestSubscribedIDs(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()
	owner := models.User{ID: uuid.NewString(), Username: "owner", PasswordHash: "x", CreatedAt: now}
	fan := models.User{ID: uuid.NewString(), Username: "fan", PasswordHash: "x", CreatedAt: now}
	for _, u := range []models.User{owner, fan} {
		if err := store.Users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	a := models.Playlist{ID: uuid.NewString(), Name: "A", OwnerID: owner.ID, Public: true, CreatedAt: now, UpdatedAt: now}
	b := models.Playlist{ID: uuid.NewString(), Name: "B", OwnerID: owner.ID, Public: true, CreatedAt: now, UpdatedAt: now}
	for _, p := range []models.Playlist{a, b} {
		if err := store.Playlists.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Playlists.Subscribe(ctx, a.ID, fan.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.Playlists.SubscribedIDs(ctx, fan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got[a.ID] || got[b.ID] || len(got) != 1 {
		t.Fatalf("SubscribedIDs = %v, want only %s", got, a.ID)
	}
}
