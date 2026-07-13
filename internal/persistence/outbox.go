package persistence

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/db"
)

// OutboxRepo is a generic durable job queue drained by a single worker. Jobs are
// keyed by `kind` (which selects a handler) and may carry an opaque payload. It
// is feature-agnostic — any subsystem can enqueue its own kinds.
type OutboxRepo struct{ *base }

// OutboxJob is a claimed queue entry. ClaimToken is the lease stamped by
// ClaimNext; Done/Backoff/Defer pass it back so they only affect the row while
// it still holds that lease (a concurrent Enqueue clears it).
type OutboxJob struct {
	ID         string
	Kind       string
	DedupeKey  string
	Payload    string
	Attempts   int
	ClaimToken string
}

// Enqueue adds a job. When dedupeKey != "" the row id is derived from
// kind+dedupeKey, so repeated enqueues for the same target collapse into one
// pending row (and reset its attempts/retry). With an empty dedupeKey every
// enqueue is a distinct job.
func (r *OutboxRepo) Enqueue(ctx context.Context, kind, dedupeKey, payload string) error {
	id := uuid.NewString()
	if dedupeKey != "" {
		id = kind + ":" + dedupeKey
	}
	_, err := r.exec(ctx,
		`INSERT INTO outbox (id, kind, dedupe_key, payload, attempts, next_retry_at, created_at)
		 VALUES (?, ?, ?, ?, 0, 0, ?)
		 ON CONFLICT (id) DO UPDATE SET payload = excluded.payload, attempts = 0, next_retry_at = 0, claim_token = ''`,
		id, kind, dedupeKey, payload, db.Millis(time.Now()))
	return err
}

// ClaimNext returns the oldest job whose retry time has arrived, or ErrNotFound
// when nothing is due. It leases the claimed row with a fresh claim token so a
// concurrent Enqueue for the same dedupe key (which resets the token) makes any
// later Done/Backoff/Defer from this claim a no-op rather than clobbering the
// re-enqueued job.
func (r *OutboxRepo) ClaimNext(ctx context.Context, now time.Time) (OutboxJob, error) {
	var j OutboxJob
	err := r.queryRow(ctx,
		`SELECT id, kind, dedupe_key, payload, attempts FROM outbox
		 WHERE next_retry_at <= ? ORDER BY next_retry_at, created_at LIMIT 1`,
		db.Millis(now)).Scan(&j.ID, &j.Kind, &j.DedupeKey, &j.Payload, &j.Attempts)
	if err == sql.ErrNoRows {
		return OutboxJob{}, ErrNotFound
	}
	if err != nil {
		return OutboxJob{}, err
	}
	j.ClaimToken = uuid.NewString()
	if _, err := r.exec(ctx, `UPDATE outbox SET claim_token = ? WHERE id = ?`, j.ClaimToken, j.ID); err != nil {
		return OutboxJob{}, err
	}
	return j, nil
}

// Backoff reschedules a failed job and counts the attempt, only while the row
// still holds the given claim token.
func (r *OutboxRepo) Backoff(ctx context.Context, id, claimToken string, nextRetry time.Time) error {
	_, err := r.exec(ctx,
		`UPDATE outbox SET attempts = attempts + 1, next_retry_at = ? WHERE id = ? AND claim_token = ?`,
		db.Millis(nextRetry), id, claimToken)
	return err
}

// Defer reschedules a job WITHOUT counting an attempt (used when a handler is not
// ready yet, e.g. a dependency is temporarily unavailable), only while the row
// still holds the given claim token.
func (r *OutboxRepo) Defer(ctx context.Context, id, claimToken string, nextRetry time.Time) error {
	_, err := r.exec(ctx, `UPDATE outbox SET next_retry_at = ? WHERE id = ? AND claim_token = ?`, db.Millis(nextRetry), id, claimToken)
	return err
}

// Done removes a completed job, only while the row still holds the given claim
// token — a concurrent re-enqueue clears the token so its fresh payload is not
// deleted by this stale completion.
func (r *OutboxRepo) Done(ctx context.Context, id, claimToken string) error {
	_, err := r.exec(ctx, `DELETE FROM outbox WHERE id = ? AND claim_token = ?`, id, claimToken)
	return err
}
