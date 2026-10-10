package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
)

// TestPlayQueueSaveFromDeviceRespectsTarget: once a device takes over, a late
// save from the device it took over from must be rejected, or that stale
// write gets broadcast and yanks every other device back to the old state.
func TestPlayQueueSaveFromDeviceRespectsTarget(t *testing.T) {
	_, store, _ := newOnDemand(t)
	ctx := context.Background()
	user := models.User{ID: uuid.NewString(), Username: "u", PasswordHash: "x", CreatedAt: time.Now()}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	svc := NewPlayQueueService(store.PlayQueues, store.Catalog, store.Annotations, nil)

	save := func(device string) error {
		return svc.SaveFromDevice(ctx, user.ID, "t1", 1000, true, device, []string{"t1"}, nil, false, "off")
	}
	if err := save("phone"); err != nil {
		t.Fatalf("no target yet, any device may save: %v", err)
	}
	if err := svc.SetTarget(ctx, user.ID, "laptop"); err != nil {
		t.Fatal(err)
	}
	if err := save("phone"); !errors.Is(err, ErrNotActiveDevice) {
		t.Fatalf("expected ErrNotActiveDevice from the old device, got %v", err)
	}
	if err := save("laptop"); err != nil {
		t.Fatalf("active device must be able to save: %v", err)
	}

	res, _ := svc.Get(ctx, user.ID)
	if res.TargetOnline {
		t.Fatal("target never connected, must not be reported online")
	}
	_, unsubscribe := svc.Subscribe(user.ID, "laptop")
	if res, _ = svc.Get(ctx, user.ID); !res.TargetOnline {
		t.Fatal("target with an open stream must be online")
	}
	unsubscribe()
	svc.presence[user.ID]["laptop"].lastSeen = time.Now().Add(-presenceTTL)
	if res, _ = svc.Get(ctx, user.ID); res.TargetOnline {
		t.Fatal("target whose stream closed past the TTL must be offline")
	}
}
