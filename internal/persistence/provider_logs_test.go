package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestProviderLogsInsertAndList(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	for _, l := range []models.ProviderLog{
		{Provider: "fma", Level: "warn", Action: "search", Message: "timeout"},
		{Provider: "fma", Level: "error", Action: "download", Message: "404"},
		{Provider: "other", Level: "error", Action: "resolve", Message: "boom"},
	} {
		if err := store.ProviderLogs.Insert(ctx, l); err != nil {
			t.Fatal(err)
		}
		// created_at has millisecond granularity; space inserts so newest-first
		// ordering is deterministic (ties are otherwise broken by random id).
		time.Sleep(2 * time.Millisecond)
	}

	got, err := store.ProviderLogs.ListByProvider(ctx, "fma", 100)
	if err != nil {
		t.Fatal(err)
	}
	// Only this provider's logs, newest first.
	if len(got) != 2 {
		t.Fatalf("want 2 logs, got %d", len(got))
	}
	if got[0].Action != "download" || got[1].Action != "search" {
		t.Fatalf("expected newest-first ordering, got %q then %q", got[0].Action, got[1].Action)
	}
	if got[0].CreatedAt.IsZero() {
		t.Fatal("createdAt not populated")
	}

	// Pruning: a cutoff in the past keeps everything; a cutoff in the future
	// (after all rows) removes everything.
	if n, err := store.ProviderLogs.PruneOlderThan(ctx, time.Now().Add(-time.Hour)); err != nil || n != 0 {
		t.Fatalf("prune past cutoff: removed %d, err %v (want 0)", n, err)
	}
	if n, err := store.ProviderLogs.PruneOlderThan(ctx, time.Now().Add(time.Hour)); err != nil || n != 3 {
		t.Fatalf("prune future cutoff: removed %d, err %v (want 3)", n, err)
	}
	left, _ := store.ProviderLogs.ListByProvider(ctx, "fma", 100)
	if len(left) != 0 {
		t.Fatalf("expected all rows pruned, got %d", len(left))
	}
}
