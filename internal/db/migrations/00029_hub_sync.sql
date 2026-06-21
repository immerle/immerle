-- +goose Up
-- +goose StatementBegin
-- Outbox of public-playlist sync jobs to push to the federation hub. One pending
-- row per playlist (external_id = local playlist id = idempotency key); a single
-- worker drains it with retry/backoff. The worker resolves upsert-vs-delete by
-- reading the playlist's current state, so no kind column is needed.
CREATE TABLE hub_outbox (
    external_id   TEXT PRIMARY KEY,
    attempts      INTEGER NOT NULL DEFAULT 0,
    next_retry_at INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);
CREATE INDEX idx_hub_outbox_retry ON hub_outbox (next_retry_at);

-- Last successfully-synced content hash per playlist, so an unchanged playlist
-- is skipped entirely (no hub calls).
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
DROP TABLE hub_outbox;
-- +goose StatementEnd
