-- +goose Up
-- +goose StatementBegin
-- One connected Bandcamp account per immerle user. There's no official OAuth,
-- so identity_enc holds the user's pasted browser session cookie, encrypted at
-- rest (core.secretBox, keyed by the instance secret) — never stored plaintext.
-- invalid_since is set the first time an import job discovers the cookie no
-- longer works (logout/password change), so the user can be prompted to
-- reconnect instead of the worker silently retrying forever.
CREATE TABLE IF NOT EXISTS bandcamp_connections (
    user_id        TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    fan_id         TEXT NOT NULL,
    identity_enc   TEXT NOT NULL,
    connected_at   INTEGER NOT NULL,
    last_synced_at INTEGER,
    invalid_since  INTEGER
);

-- One row per user-triggered import of one purchased item (album or track).
-- The unique index makes re-clicking "import" on the same item idempotent —
-- see BandcampImportRepo.Enqueue.
CREATE TABLE IF NOT EXISTS bandcamp_import_jobs (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sale_item_type  TEXT NOT NULL,
    sale_item_id    TEXT NOT NULL,
    item_type       TEXT NOT NULL,
    artist_name     TEXT NOT NULL,
    item_title      TEXT NOT NULL,
    format          TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL,
    track_ids       TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    attempts        INTEGER NOT NULL DEFAULT 0,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bandcamp_jobs_user_item ON bandcamp_import_jobs (user_id, sale_item_type, sale_item_id);
CREATE INDEX IF NOT EXISTS idx_bandcamp_jobs_status ON bandcamp_import_jobs (status, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE bandcamp_import_jobs;
DROP TABLE bandcamp_connections;
-- +goose StatementEnd
