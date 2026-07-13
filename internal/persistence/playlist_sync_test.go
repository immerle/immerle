package persistence_test

import (
	"context"
	"testing"

	"github.com/immerle/immerle/internal/testutil"
)

func TestPlaylistSyncLastPayload(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	if payload, version, err := store.PlaylistSync.LastPayload(ctx, "pl1"); err != nil || payload != "" || version != "" {
		t.Fatalf("want empty payload/version before first sync, got %q %q err %v", payload, version, err)
	}

	if err := store.PlaylistSync.SetPayload(ctx, "pl1", `{"name":"foo"}`, "2026-07-13T10:00:00Z"); err != nil {
		t.Fatal(err)
	}
	payload, version, err := store.PlaylistSync.LastPayload(ctx, "pl1")
	if err != nil {
		t.Fatal(err)
	}
	if payload != `{"name":"foo"}` || version != "2026-07-13T10:00:00Z" {
		t.Fatalf("got payload %q version %q, want the just-set values", payload, version)
	}

	// SetPayload upserts without disturbing the unrelated hash column.
	if err := store.PlaylistSync.Set(ctx, "pl1", "somehash"); err != nil {
		t.Fatal(err)
	}
	if err := store.PlaylistSync.SetPayload(ctx, "pl1", `{"name":"bar"}`, "2026-07-13T11:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if hash, err := store.PlaylistSync.Hash(ctx, "pl1"); err != nil || hash != "somehash" {
		t.Fatalf("hash should survive SetPayload, got %q err %v", hash, err)
	}
	payload, version, err = store.PlaylistSync.LastPayload(ctx, "pl1")
	if err != nil {
		t.Fatal(err)
	}
	if payload != `{"name":"bar"}` || version != "2026-07-13T11:00:00Z" {
		t.Fatalf("got payload %q version %q, want the updated values", payload, version)
	}
}

func TestFeedCursor(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	if v, err := store.FeedCursors.Get(ctx, "instance-a"); err != nil || v != "" {
		t.Fatalf("want empty cursor before first set, got %q err %v", v, err)
	}

	if err := store.FeedCursors.Set(ctx, "instance-a", "2026-07-13T10:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if v, err := store.FeedCursors.Get(ctx, "instance-a"); err != nil || v != "2026-07-13T10:00:00Z" {
		t.Fatalf("got %q err %v, want the just-set version", v, err)
	}

	// A different source instance is tracked independently.
	if v, err := store.FeedCursors.Get(ctx, "instance-b"); err != nil || v != "" {
		t.Fatalf("want empty cursor for a different instance, got %q err %v", v, err)
	}

	if err := store.FeedCursors.Set(ctx, "instance-a", "2026-07-13T11:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if v, err := store.FeedCursors.Get(ctx, "instance-a"); err != nil || v != "2026-07-13T11:00:00Z" {
		t.Fatalf("got %q err %v, want the updated version", v, err)
	}
}
