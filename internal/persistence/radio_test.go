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
	// A built-in no longer in the embedded list must be pruned on the next seed.
	orphan := models.RadioStation{ID: "builtin:gone", Name: "Gone", StreamURL: "https://x/s", Country: "fr", Builtin: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.Radio.Create(ctx, orphan); err != nil {
		t.Fatal(err)
	}
	if err := store.Radio.EnsureBuiltins(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Radio.Get(ctx, orphan.ID); err != persistence.ErrNotFound {
		t.Fatalf("orphan built-in not pruned: err=%v", err)
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
		if s.Country == "" {
			t.Fatalf("seeded station %q has no country", s.Name)
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

func TestRadioLikes(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	user := models.User{ID: "u1", Username: "u", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	st := models.RadioStation{ID: "builtin:test", Name: "Test", StreamURL: "https://x/s", Country: "fr", CreatedAt: now, UpdatedAt: now}
	if err := store.Radio.Create(ctx, st); err != nil {
		t.Fatal(err)
	}

	// Like, then verify it shows up for the user and nowhere in track stars.
	if err := store.Radio.SetLiked(ctx, user.ID, st.ID, true); err != nil {
		t.Fatal(err)
	}
	liked, err := store.Radio.LikedIDs(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !liked[st.ID] {
		t.Fatal("station not marked liked")
	}
	if starred, _ := store.Annotations.ListStarred(ctx, user.ID, models.ItemTrack); len(starred) != 0 {
		t.Fatalf("radio like leaked into track stars: %v", starred)
	}
	withLike, _ := store.Radio.ListForUser(ctx, user.ID)
	if len(withLike) != 1 || !withLike[0].Liked {
		t.Fatalf("ListForUser did not reflect like: %+v", withLike)
	}

	// Unlike clears it.
	if err := store.Radio.SetLiked(ctx, user.ID, st.ID, false); err != nil {
		t.Fatal(err)
	}
	if liked, _ := store.Radio.LikedIDs(ctx, user.ID); liked[st.ID] {
		t.Fatal("station still liked after unlike")
	}
}
