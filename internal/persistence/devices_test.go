package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestTouchSeenBuffersThenFlushPersists(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	user := models.User{ID: uuid.NewString(), Username: "alice", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	dev := models.Device{ID: uuid.NewString(), UserID: user.ID, Name: "Pixel 8", CreatedAt: now}
	if err := store.Devices.Create(ctx, dev); err != nil {
		t.Fatal(err)
	}

	touchedAt := now.Add(time.Minute).Truncate(time.Millisecond)
	if err := store.Devices.TouchSeen(ctx, dev, "1.2.3.4", touchedAt); err != nil {
		t.Fatal(err)
	}

	// Before any flush, reads still see the buffered touch (overlay).
	got, err := store.Devices.Get(ctx, dev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastSeenAt == nil || !got.LastSeenAt.Equal(touchedAt) || got.LastIP != "1.2.3.4" {
		t.Fatalf("Get before flush should reflect the buffered touch, got %+v", got)
	}

	if err := store.Devices.FlushSeen(ctx); err != nil {
		t.Fatal(err)
	}

	// After flush the buffer is drained; a fresh read must come from the DB and
	// still match, proving FlushSeen actually persisted it.
	got, err = store.Devices.Get(ctx, dev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastSeenAt == nil || !got.LastSeenAt.Equal(touchedAt) || got.LastIP != "1.2.3.4" {
		t.Fatalf("Get after flush should reflect the persisted touch, got %+v", got)
	}

	list, err := store.Devices.ListByUser(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].LastIP != "1.2.3.4" {
		t.Fatalf("ListByUser should also reflect the persisted touch, got %+v", list)
	}
}
