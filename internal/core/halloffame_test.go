package core

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
)

// TestHallOfFameAddResolvesRemoteTrack covers a real bug: a track fetched
// on-the-fly from an on-demand provider (a "remote:" id, not yet a row in
// `tracks`) couldn't be added to a Hall of Fame — hall_of_fame_entries has a
// foreign key on track_id, so the insert failed deep in the persistence layer
// with an opaque "FOREIGN KEY constraint failed". Add/SetOrder must resolve
// (download) such ids first, the same way PlaylistService already does.
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
