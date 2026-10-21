package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// TestShareCreateChecksAccess: a share link exposes the item's tracks to
// anyone, so a user may only share what they can see.
func TestShareCreateChecksAccess(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	alice := models.User{ID: uuid.NewString(), Username: "alice", PasswordHash: "x", CreatedAt: now}
	bob := models.User{ID: uuid.NewString(), Username: "bob", PasswordHash: "x", CreatedAt: now}
	for _, u := range []models.User{alice, bob} {
		if err := store.Users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	private := models.Playlist{ID: uuid.NewString(), Name: "Alice only", OwnerID: alice.ID, CreatedAt: now, UpdatedAt: now}
	public := models.Playlist{ID: uuid.NewString(), Name: "For all", OwnerID: alice.ID, Public: true, CreatedAt: now, UpdatedAt: now}
	for _, p := range []models.Playlist{private, public} {
		if err := store.Playlists.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	svc := NewShareService(store.Shares, store.Catalog, store.Playlists)

	if _, err := svc.Create(ctx, bob, private.ID, "", nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("sharing someone else's private playlist: want ErrForbidden, got %v", err)
	}
	if _, err := svc.Create(ctx, alice, private.ID, "", nil); err != nil {
		t.Fatalf("owner sharing their private playlist: %v", err)
	}
	if swe, err := svc.Create(ctx, bob, public.ID, "", nil); err != nil || swe.Share.ItemType != models.ItemPlaylist {
		t.Fatalf("sharing a public playlist: %+v, %v", swe.Share, err)
	}
	if _, err := svc.Create(ctx, alice, uuid.NewString(), "", nil); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("sharing an unknown id: want ErrNotFound, got %v", err)
	}
	if shares, _ := store.Shares.ListByUser(ctx, bob.ID); len(shares) != 1 {
		t.Fatalf("bob should own exactly the public-playlist share, got %d", len(shares))
	}
}
