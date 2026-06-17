package persistence_test

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

func TestFriendAcceptRequiresPendingRequest(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	a := models.User{ID: uuid.NewString(), Username: "alice", PasswordHash: "x", CreatedAt: now}
	b := models.User{ID: uuid.NewString(), Username: "bob", PasswordHash: "x", CreatedAt: now}
	for _, u := range []models.User{a, b} {
		if err := store.Users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
	}

	// No request from a to b: b accepting must fail and forge nothing.
	if err := store.Friends.Accept(ctx, a.ID, b.ID, uuid.NewString()); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("Accept without pending request = %v, want ErrNotFound", err)
	}
	if ok, _ := store.Friends.AreFriends(ctx, b.ID, a.ID); ok {
		t.Fatal("friendship was forged without a pending request")
	}

	// With a real pending request, accept succeeds both ways.
	if err := store.Friends.Request(ctx, models.Friendship{
		ID: uuid.NewString(), UserID: a.ID, FriendID: b.ID,
		Status: models.FriendPending, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Friends.Accept(ctx, a.ID, b.ID, uuid.NewString()); err != nil {
		t.Fatalf("Accept with pending request: %v", err)
	}
	if ok, _ := store.Friends.AreFriends(ctx, a.ID, b.ID); !ok {
		t.Fatal("a->b not accepted")
	}
	if ok, _ := store.Friends.AreFriends(ctx, b.ID, a.ID); !ok {
		t.Fatal("reciprocal b->a edge not created")
	}
}
