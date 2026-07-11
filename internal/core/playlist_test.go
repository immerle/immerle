package core

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

// TestFederatedPlaylistNotMutableByNominalOwner covers a real bug: a federated
// playlist is attributed to a nominal local owner (whichever admin the sync
// process picked) purely so the row satisfies the owner_id FK — that
// attribution must never grant real ownership. The nominal owner must not be
// able to delete, re-cover or add collaborators to it; they can only
// unsubscribe, like anyone else.
func TestFederatedPlaylistNotMutableByNominalOwner(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	p := models.Playlist{
		ID: uuid.NewString(), Name: "Hub Picks", OwnerID: owner.ID, Public: true, Federated: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Playlists.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	svc := NewPlaylistService(store.Playlists, store.Annotations, nil, nil)

	// Not deletable by the nominal owner: not subscribed, so forbidden outright.
	if err := svc.Delete(ctx, owner, p.ID); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden deleting a federated playlist, got %v", err)
	}
	if _, err := store.Playlists.Get(ctx, p.ID); err != nil {
		t.Fatalf("federated playlist should still exist: %v", err)
	}

	// Not re-coverable.
	if _, err := svc.CoverTarget(ctx, owner, p.ID); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden setting cover on a federated playlist, got %v", err)
	}

	// Once subscribed, Delete unsubscribes instead of deleting the row.
	if err := store.Playlists.Subscribe(ctx, p.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, owner, p.ID); err != nil {
		t.Fatalf("unsubscribe-via-delete should succeed once subscribed: %v", err)
	}
	if _, err := store.Playlists.Get(ctx, p.ID); err != nil {
		t.Fatalf("federated playlist should still exist after unsubscribing: %v", err)
	}
	if subscribed, _ := store.Playlists.IsSubscribed(ctx, p.ID, owner.ID); subscribed {
		t.Fatal("expected the subscription to be gone")
	}
}
