-- +goose Up
-- +goose StatementBegin
-- Retry of 00038: goose tracks migrations by version number, not content, so
-- an install where version 38 was already recorded (under an earlier,
-- abandoned "ranked playlist" approach) never got the hall_of_fame tables
-- from the rewritten 00038. IF NOT EXISTS makes this a no-op on installs
-- where 00038 already created them correctly.
CREATE TABLE IF NOT EXISTS hall_of_fame (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS hall_of_fame_entries (
    hall_of_fame_id TEXT NOT NULL REFERENCES hall_of_fame(id) ON DELETE CASCADE,
    track_id        TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position        INTEGER NOT NULL,
    comment         TEXT NOT NULL DEFAULT '',
    added_at        INTEGER NOT NULL,
    PRIMARY KEY (hall_of_fame_id, position)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_hall_of_fame_entries_track ON hall_of_fame_entries (hall_of_fame_id, track_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS hall_of_fame_entries;
DROP TABLE IF EXISTS hall_of_fame;
-- +goose StatementEnd
