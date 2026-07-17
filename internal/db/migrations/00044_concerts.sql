-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN city TEXT NOT NULL DEFAULT '';

-- Concert discovery: one row per (user, matched event), found by searching
-- Ticketmaster/Skiddle for the user's top-listened artists near their city.
-- dismissed_at is never touched by a resync (see ConcertRepo.Upsert) — once a
-- user closes the banner for an event it stays closed even after the next
-- daily sync runs again.
CREATE TABLE IF NOT EXISTS concerts (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source           TEXT NOT NULL,
    source_event_id  TEXT NOT NULL,
    artist_name      TEXT NOT NULL,
    event_name       TEXT NOT NULL,
    venue            TEXT NOT NULL DEFAULT '',
    city             TEXT NOT NULL DEFAULT '',
    start_time       INTEGER NOT NULL,
    url              TEXT NOT NULL DEFAULT '',
    dismissed_at     INTEGER,
    created_at       INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_concerts_user_event ON concerts (user_id, source, source_event_id);
CREATE INDEX IF NOT EXISTS idx_concerts_user_active ON concerts (user_id, start_time);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE concerts;
ALTER TABLE users DROP COLUMN city;
-- +goose StatementEnd
