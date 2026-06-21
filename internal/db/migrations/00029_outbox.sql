-- +goose Up
-- +goose StatementBegin
-- Generic durable outbox of async jobs, drained by a single worker with
-- retry/backoff. `kind` selects the registered handler; `dedupe_key` (optional)
-- collapses repeated jobs for the same target into one pending row (the id is
-- derived from kind+dedupe_key so a re-enqueue upserts). Not tied to any feature.
CREATE TABLE outbox (
    id            TEXT PRIMARY KEY,
    kind          TEXT NOT NULL,
    dedupe_key    TEXT NOT NULL DEFAULT '',
    payload       TEXT NOT NULL DEFAULT '',
    attempts      INTEGER NOT NULL DEFAULT 0,
    next_retry_at INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);
CREATE INDEX idx_outbox_due ON outbox (next_retry_at);

-- Federation playlist-sync state (consumer of the outbox): last synced content
-- hash per playlist, so an unchanged playlist is skipped with no hub call.
CREATE TABLE playlist_sync (
    playlist_id       TEXT PRIMARY KEY,
    last_payload_hash TEXT NOT NULL DEFAULT '',
    last_synced_at    INTEGER NOT NULL DEFAULT 0
);

-- Content-addressed covers confirmed present on the hub (sha256 hex), so a known
-- cover skips the /covers/missing probe and the upload.
CREATE TABLE cover_uploads (
    sha256     TEXT PRIMARY KEY,
    created_at INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE cover_uploads;
DROP TABLE playlist_sync;
DROP TABLE outbox;
-- +goose StatementEnd
