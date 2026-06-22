// Package outbox is a generic, durable async job queue: subsystems enqueue jobs
// (keyed by a `kind` that selects a handler) and a single worker drains them in
// its own goroutine with retry/backoff. It is feature-agnostic — federation
// playlist sync is one consumer; anything needing reliable background work can
// register its own kind.
package outbox

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/immerle/immerle/internal/persistence"
)

const (
	tick          = 15 * time.Second // rescan cadence absent a wake
	notReadyDelay = 30 * time.Second // defer when a handler is not ready
	baseBackoff   = 5 * time.Second
	maxBackoff    = 30 * time.Minute
)

// ErrNotReady tells the worker to retry a job later WITHOUT counting an attempt
// (a transient dependency is unavailable — e.g. the hub isn't linked yet).
var ErrNotReady = errors.New("outbox: not ready")

// retryAfter wraps an error with an explicit retry delay.
type retryAfter struct {
	d   time.Duration
	err error
}

func (e *retryAfter) Error() string { return e.err.Error() }
func (e *retryAfter) Unwrap() error { return e.err }

// RetryAfter wraps err so the worker retries the job after d instead of the
// default exponential backoff (still counts as an attempt). Use for explicit
// "retry-after" signals such as a rate-limit response.
func RetryAfter(d time.Duration, err error) error { return &retryAfter{d: d, err: err} }

// Handler processes one job of a registered kind. Return nil on success (the job
// is removed), ErrNotReady to defer without penalty, RetryAfter(d, err) to set
// the delay, or any other error for exponential backoff.
type Handler func(ctx context.Context, job persistence.OutboxJob) error

// Worker drains the outbox, dispatching each job to the handler registered for
// its kind. Run it once in its own goroutine.
type Worker struct {
	repo     *persistence.OutboxRepo
	logger   *slog.Logger
	handlers map[string]Handler
	wake     chan struct{}
}

// NewWorker builds a worker over the outbox repo.
func NewWorker(repo *persistence.OutboxRepo, logger *slog.Logger) *Worker {
	return &Worker{repo: repo, logger: logger, handlers: map[string]Handler{}, wake: make(chan struct{}, 1)}
}

// Register binds a handler to a job kind. Call during setup, before Run (the
// handler map is not guarded for concurrent writes).
func (w *Worker) Register(kind string, h Handler) { w.handlers[kind] = h }

// Enqueue adds a job and wakes the worker. dedupeKey (optional) collapses repeats
// for the same target into one pending row.
func (w *Worker) Enqueue(ctx context.Context, kind, dedupeKey, payload string) error {
	if err := w.repo.Enqueue(ctx, kind, dedupeKey, payload); err != nil {
		return err
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return nil
}

// Run drains the queue on enqueue or on a periodic tick until ctx is done.
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		w.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-t.C:
		}
	}
}

// drain processes every due job once. Failed jobs are rescheduled (future
// next_retry_at) so they are not re-claimed this round; the loop ends on the
// first ErrNotFound (queue empty / nothing due) or an ErrNotReady defer.
func (w *Worker) drain(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		job, err := w.repo.ClaimNext(ctx, time.Now())
		if err != nil {
			return
		}
		h := w.handlers[job.Kind]
		if h == nil {
			w.logger.Warn("outbox: no handler for kind; dropping job", "kind", job.Kind, "id", job.ID)
			_ = w.repo.Done(ctx, job.ID)
			continue
		}
		switch err := h(ctx, job); {
		case err == nil:
			_ = w.repo.Done(ctx, job.ID)
		case errors.Is(err, ErrNotReady):
			_ = w.repo.Defer(ctx, job.ID, time.Now().Add(notReadyDelay))
			return // dependency down → ease off this round
		default:
			d := backoff(job.Attempts)
			var ra *retryAfter
			if errors.As(err, &ra) {
				d = ra.d
			}
			w.logger.Warn("outbox: job failed; will retry", "kind", job.Kind, "id", job.ID, "attempt", job.Attempts+1, "retryIn", d, "error", err)
			_ = w.repo.Backoff(ctx, job.ID, time.Now().Add(d))
		}
	}
}

// backoff is an exponential retry delay (5s, 10s, 20s … capped at maxBackoff).
func backoff(attempts int) time.Duration {
	shift := attempts
	if shift > 8 {
		shift = 8
	}
	d := baseBackoff << uint(shift)
	if d <= 0 || d > maxBackoff {
		d = maxBackoff
	}
	return d
}
