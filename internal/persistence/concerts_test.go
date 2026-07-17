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

func newTestUser(t *testing.T, store *persistence.Store) models.User {
	t.Helper()
	u := models.User{ID: uuid.NewString(), Username: uuid.NewString(), PasswordHash: "x", CreatedAt: time.Now()}
	if err := store.Users.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func newConcert(userID string, startTime time.Time) models.Concert {
	return models.Concert{
		ID: uuid.NewString(), UserID: userID, Source: "ticketmaster", SourceEventID: "evt-1",
		ArtistName: "Daft Punk", EventName: "Daft Punk Live", StartTime: startTime,
	}
}

// TestConcertRepoUpsertDedupesAndPreservesDismissal covers the core guarantee
// a resync depends on: a second Upsert for the same (user, source, event) is
// a no-op, and in particular never resurrects a dismissal.
func TestConcertRepoUpsertDedupesAndPreservesDismissal(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	user := newTestUser(t, store)
	now := time.Now()
	start := now.Add(48 * time.Hour)

	c := newConcert(user.ID, start)
	created, err := store.Concerts.Upsert(ctx, c)
	if err != nil || !created {
		t.Fatalf("first Upsert: created=%v err=%v, want created=true", created, err)
	}

	list, err := store.Concerts.ListActive(ctx, user.ID, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("ListActive = %d concerts, want 1", len(list))
	}
	if found, err := store.Concerts.Dismiss(ctx, user.ID, list[0].ID); err != nil || !found {
		t.Fatalf("Dismiss: found=%v err=%v, want found=true", found, err)
	}

	// A resync (same source + source event id, different row id like a real
	// sync would generate) must not resurrect the dismissed row.
	resynced := newConcert(user.ID, start)
	created, err = store.Concerts.Upsert(ctx, resynced)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("Upsert reported created=true on a conflicting (user, source, event) — dedupe key isn't working")
	}
	list, err = store.Concerts.ListActive(ctx, user.ID, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("ListActive after resync = %d concerts, want 0 (still dismissed)", len(list))
	}
}

// TestConcertRepoListActiveFiltersPastAndOrders covers ListActive's two
// filters (dismissed, past start_time) and its soonest-first ordering.
func TestConcertRepoListActiveFiltersPastAndOrders(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	user := newTestUser(t, store)
	now := time.Now()

	past := models.Concert{ID: uuid.NewString(), UserID: user.ID, Source: "ticketmaster", SourceEventID: "past",
		ArtistName: "A", EventName: "Past show", StartTime: now.Add(-24 * time.Hour)}
	soon := models.Concert{ID: uuid.NewString(), UserID: user.ID, Source: "ticketmaster", SourceEventID: "soon",
		ArtistName: "B", EventName: "Soon show", StartTime: now.Add(24 * time.Hour)}
	later := models.Concert{ID: uuid.NewString(), UserID: user.ID, Source: "skiddle", SourceEventID: "later",
		ArtistName: "C", EventName: "Later show", StartTime: now.Add(72 * time.Hour)}
	for _, c := range []models.Concert{past, soon, later} {
		if _, err := store.Concerts.Upsert(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	list, err := store.Concerts.ListActive(ctx, user.ID, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].EventName != "Soon show" || list[1].EventName != "Later show" {
		t.Fatalf("ListActive = %+v, want [Soon show, Later show] (past excluded, soonest first)", list)
	}
}

// TestConcertRepoDismissScopedToOwner covers the ownership guard: dismissing
// by id alone (no owner check) would let any user close another's banner.
func TestConcertRepoDismissScopedToOwner(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	owner := newTestUser(t, store)
	other := newTestUser(t, store)

	c := newConcert(owner.ID, time.Now().Add(24*time.Hour))
	if _, err := store.Concerts.Upsert(ctx, c); err != nil {
		t.Fatal(err)
	}
	list, err := store.Concerts.ListActive(ctx, owner.ID, time.Now(), 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("setup: ListActive = %+v, err=%v", list, err)
	}

	if found, err := store.Concerts.Dismiss(ctx, other.ID, list[0].ID); err != nil || found {
		t.Fatalf("Dismiss(wrong owner): found=%v err=%v, want found=false", found, err)
	}
	if found, err := store.Concerts.Dismiss(ctx, owner.ID, list[0].ID); err != nil || !found {
		t.Fatalf("Dismiss(owner): found=%v err=%v, want found=true", found, err)
	}
}
