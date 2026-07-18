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

// TestBandcampConnectionRepoUpsertGetReconnectDelete covers the connection
// lifecycle: connect, read back, reconnect overwrites in place, disconnect
// removes it.
func TestBandcampConnectionRepoUpsertGetReconnectDelete(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	user := newTestUser(t, store)

	if _, err := store.BandcampConns.Get(ctx, user.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("Get before connect = %v, want ErrNotFound", err)
	}

	now := time.Now()
	if err := store.BandcampConns.Upsert(ctx, models.BandcampConnection{
		UserID: user.ID, FanID: "111", IdentityEnc: "enc-1", ConnectedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.BandcampConns.Get(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FanID != "111" || got.IdentityEnc != "enc-1" {
		t.Fatalf("Get = %+v, want fan_id=111 identity_enc=enc-1", got)
	}

	// Simulate a job discovering the cookie is dead, then a reconnect.
	if err := store.BandcampConns.MarkInvalid(ctx, user.ID, now); err != nil {
		t.Fatal(err)
	}
	if got, err = store.BandcampConns.Get(ctx, user.ID); err != nil || got.InvalidSince == nil {
		t.Fatalf("Get after MarkInvalid = %+v, err=%v, want InvalidSince set", got, err)
	}
	if err := store.BandcampConns.Upsert(ctx, models.BandcampConnection{
		UserID: user.ID, FanID: "222", IdentityEnc: "enc-2", ConnectedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = store.BandcampConns.Get(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FanID != "222" || got.IdentityEnc != "enc-2" || got.InvalidSince != nil {
		t.Fatalf("Get after reconnect = %+v, want fan_id=222 identity_enc=enc-2 InvalidSince=nil", got)
	}

	if err := store.BandcampConns.Delete(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BandcampConns.Get(ctx, user.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrNotFound", err)
	}
}

func newBandcampJob(userID, saleItemID string) models.BandcampImportJob {
	now := time.Now()
	return models.BandcampImportJob{
		ID: uuid.NewString(), UserID: userID, SaleItemType: "p", SaleItemID: saleItemID,
		ItemType: "album", ArtistName: "Pinkfong", ItemTitle: "Baby Shark",
		Status: models.BandcampQueued, CreatedAt: now, UpdatedAt: now,
	}
}

// TestBandcampImportRepoEnqueueIsIdempotent covers the queue-level dedup: a
// second Enqueue for the same (user, sale item) returns the existing job
// instead of creating a duplicate.
func TestBandcampImportRepoEnqueueIsIdempotent(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	user := newTestUser(t, store)

	first, created, err := store.BandcampImports.Enqueue(ctx, newBandcampJob(user.ID, "123"))
	if err != nil || !created {
		t.Fatalf("first Enqueue: created=%v err=%v, want created=true", created, err)
	}

	again, created, err := store.BandcampImports.Enqueue(ctx, newBandcampJob(user.ID, "123"))
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("second Enqueue reported created=true — dedupe key isn't working")
	}
	if again.ID != first.ID {
		t.Fatalf("second Enqueue returned job %s, want the original %s", again.ID, first.ID)
	}

	list, err := store.BandcampImports.ListByUser(ctx, user.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("ListByUser = %d jobs, want 1", len(list))
	}
}

// TestBandcampImportRepoClaimNextAndComplete covers the worker's claim/complete
// cycle: ClaimNext flips status to running and bumps attempts, Complete
// records the resulting track ids.
func TestBandcampImportRepoClaimNextAndComplete(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	user := newTestUser(t, store)

	job, _, err := store.BandcampImports.Enqueue(ctx, newBandcampJob(user.ID, "456"))
	if err != nil {
		t.Fatal(err)
	}

	claimed, err := store.BandcampImports.ClaimNext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != job.ID || claimed.Status != models.BandcampRunning || claimed.Attempts != 1 {
		t.Fatalf("ClaimNext = %+v, want id=%s status=running attempts=1", claimed, job.ID)
	}

	// Queue is now empty.
	if _, err := store.BandcampImports.ClaimNext(ctx); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("ClaimNext on empty queue = %v, want ErrNotFound", err)
	}

	trackIDs := []string{uuid.NewString(), uuid.NewString()}
	if err := store.BandcampImports.Complete(ctx, job.ID, trackIDs); err != nil {
		t.Fatal(err)
	}
	got, err := store.BandcampImports.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.BandcampCompleted || len(got.TrackIDs) != 2 || got.TrackIDs[0] != trackIDs[0] || got.TrackIDs[1] != trackIDs[1] {
		t.Fatalf("Get after Complete = %+v, want status=completed trackIds=%v", got, trackIDs)
	}
}

// TestBandcampImportRepoFailRequeuesOrFails covers Fail's requeue branch (back
// to 'queued' for a retry) vs its terminal branch ('failed').
func TestBandcampImportRepoFailRequeuesOrFails(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	user := newTestUser(t, store)

	job, _, err := store.BandcampImports.Enqueue(ctx, newBandcampJob(user.ID, "789"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BandcampImports.ClaimNext(ctx); err != nil {
		t.Fatal(err)
	}

	if err := store.BandcampImports.Fail(ctx, job.ID, "temporary error", true); err != nil {
		t.Fatal(err)
	}
	got, err := store.BandcampImports.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.BandcampQueued || got.Error != "temporary error" {
		t.Fatalf("Get after Fail(requeue=true) = %+v, want status=queued error=\"temporary error\"", got)
	}

	if _, err := store.BandcampImports.ClaimNext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.BandcampImports.Fail(ctx, job.ID, "permanent error", false); err != nil {
		t.Fatal(err)
	}
	got, err = store.BandcampImports.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.BandcampFailed || got.Error != "permanent error" {
		t.Fatalf("Get after Fail(requeue=false) = %+v, want status=failed error=\"permanent error\"", got)
	}
}

// TestBandcampImportRepoRequeueStaleResetsRunning covers crash recovery: a job
// stuck in 'running' (e.g. after a crash) is reset back to 'queued'.
func TestBandcampImportRepoRequeueStaleResetsRunning(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	user := newTestUser(t, store)

	job, _, err := store.BandcampImports.Enqueue(ctx, newBandcampJob(user.ID, "999"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BandcampImports.ClaimNext(ctx); err != nil {
		t.Fatal(err)
	}

	if err := store.BandcampImports.RequeueStale(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.BandcampImports.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.BandcampQueued {
		t.Fatalf("Get after RequeueStale = %+v, want status=queued", got)
	}
}
