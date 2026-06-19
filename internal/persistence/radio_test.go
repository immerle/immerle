package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

func TestRadioBuiltinsAndCRUD(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	// Seeding is idempotent: running twice yields the same set.
	if err := store.Radio.EnsureBuiltins(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Radio.EnsureBuiltins(ctx); err != nil {
		t.Fatal(err)
	}
	seeded, err := store.Radio.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeded) == 0 {
		t.Fatal("no built-in stations seeded")
	}
	builtinCount := len(seeded)
	for _, s := range seeded {
		if !s.Builtin {
			t.Fatalf("seeded station %q not flagged builtin", s.Name)
		}
		// Built-ins are protected from deletion.
		if err := store.Radio.Delete(ctx, s.ID); err != nil {
			t.Fatal(err)
		}
	}
	if after, _ := store.Radio.List(ctx); len(after) != builtinCount {
		t.Fatalf("built-ins deleted: %d remain, want %d", len(after), builtinCount)
	}

	// Custom stations are creatable and deletable.
	now := time.Now()
	st := models.RadioStation{ID: persistence.NewStationID(), Name: "My Stream", StreamURL: "https://example.com/stream", CreatedAt: now, UpdatedAt: now}
	if err := store.Radio.Create(ctx, st); err != nil {
		t.Fatal(err)
	}
	if err := store.Radio.Delete(ctx, st.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Radio.Get(ctx, st.ID); err != persistence.ErrNotFound {
		t.Fatalf("custom station not deleted: err=%v", err)
	}
}
