package core

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
)

// TestHallOfFameAddResolvesRemoteTrack: a "remote:" track id (no row in
// `tracks` yet) used to fail with an opaque FK constraint error, since
// hall_of_fame_entries has a foreign key on track_id. Add/SetOrder must
// resolve such ids first, like PlaylistService already does.
func TestHallOfFameAddResolvesRemoteTrack(t *testing.T) {
	onDemand, store, _ := newOnDemand(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "owner", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	svc := NewHallOfFameService(store.HallOfFame, onDemand)

	remote, err := onDemand.RemoteSearch(ctx, "Remote", 10)
	if err != nil || len(remote) != 1 {
		t.Fatalf("remote search: %v %+v", err, remote)
	}
	remoteID := remote[0].ID
	if !IsRemoteID(remoteID) {
		t.Fatalf("expected a remote id, got %q", remoteID)
	}

	if err := svc.Add(ctx, owner.ID, remoteID); err != nil {
		t.Fatalf("add a remote track: %v", err)
	}
	d, err := svc.Get(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Entries) != 1 || IsRemoteID(d.Entries[0].Track.ID) || d.Entries[0].Track.ID == "" {
		t.Fatalf("expected the track resolved to a real local id, got %+v", d.Entries)
	}

	// SetOrder resolves too (defense in depth, even though callers normally
	// pass back already-resolved ids from a prior Get).
	if err := svc.SetOrder(ctx, owner.ID, []string{remoteID}); err != nil {
		t.Fatalf("set order with a remote track: %v", err)
	}
	d, err = svc.Get(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Entries) != 1 || IsRemoteID(d.Entries[0].Track.ID) {
		t.Fatalf("expected SetOrder to resolve the remote id too, got %+v", d.Entries)
	}
}
