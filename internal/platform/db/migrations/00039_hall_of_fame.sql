-- +goose Up
-- +goose StatementBegin
-- Hall of Fame: a user's personal top-tracks ranking. A dedicated table (not a
-- flag on playlists) — exactly one per user, auto-created on first access (see
-- core.HallOfFameService.Get).
CREATE TABLE IF NOT EXISTS hall_of_fame (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- One row per ranked track. track_id is unique per hall_of_fame_id: a track
-- can only appear once in a ranking. comment is the personal nostalgia note
-- ("listened to this in college"). Reordering/adding/removing all go through
-- HallOfFameRepo.ReplaceEntries, which deletes and reinserts every row but
-- carries each track's existing comment forward — so notes survive a reorder
-- despite the position-keyed primary key.
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
DROP TABLE hall_of_fame_entries;
DROP TABLE hall_of_fame;
-- +goose StatementEnd
