-- +goose Up
-- +goose StatementBegin
-- claim_token leases a claimed job to one worker pass. ClaimNext stamps a fresh
-- token; Done/Backoff/Defer only affect the row while the token still matches.
-- A concurrent Enqueue for the same dedupe key resets the token (see Enqueue),
-- so a stale Done from an earlier claim becomes a no-op instead of deleting the
-- freshly re-enqueued payload.
ALTER TABLE outbox ADD COLUMN claim_token TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE outbox DROP COLUMN claim_token;
-- +goose StatementEnd
