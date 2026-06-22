package outbox

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestWorkerDispatchesAndCompletes(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore(t)
	w := NewWorker(store.Outbox, testLogger())

	var seen []string
	w.Register("greet", func(_ context.Context, job persistence.OutboxJob) error {
		seen = append(seen, job.DedupeKey+":"+job.Payload)
		return nil
	})

	if err := w.Enqueue(ctx, "greet", "a", "hello"); err != nil {
		t.Fatal(err)
	}
	// Re-enqueue same (kind,key) collapses to one pending row (payload updated).
	if err := w.Enqueue(ctx, "greet", "a", "hi"); err != nil {
		t.Fatal(err)
	}
	w.drain(ctx)

	if len(seen) != 1 || seen[0] != "a:hi" {
		t.Fatalf("handler calls = %v, want one [a:hi]", seen)
	}
	if _, err := store.Outbox.ClaimNext(ctx, time.Now()); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("queue should be empty after success")
	}
}

func TestWorkerUnknownKindDropped(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore(t)
	w := NewWorker(store.Outbox, testLogger())
	_ = w.Enqueue(ctx, "nope", "x", "")
	w.drain(ctx)
	if _, err := store.Outbox.ClaimNext(ctx, time.Now()); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("a job with no handler should be dropped")
	}
}

func TestWorkerNotReadyDefersWithoutAttempt(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore(t)
	w := NewWorker(store.Outbox, testLogger())

	calls := 0
	w.Register("later", func(_ context.Context, _ persistence.OutboxJob) error {
		calls++
		return ErrNotReady
	})
	_ = w.Enqueue(ctx, "later", "k", "")
	w.drain(ctx)

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	// Deferred to the future (no attempt counted) → not claimable now, claimable later.
	if _, err := store.Outbox.ClaimNext(ctx, time.Now()); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatal("not-ready job should be deferred to the future")
	}
	job, err := store.Outbox.ClaimNext(ctx, time.Now().Add(time.Hour))
	if err != nil || job.Attempts != 0 {
		t.Fatalf("deferred job should remain with 0 attempts: job=%+v err=%v", job, err)
	}
}
