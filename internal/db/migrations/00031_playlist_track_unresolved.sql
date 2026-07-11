-- +goose Up
-- +goose StatementBegin
-- track_id becomes nullable (ON DELETE SET NULL instead of CASCADE) and gains
-- mbid/artist/title/album: a federated playlist entry that hasn't been matched
-- to a local track yet — or whose match was later removed from the library —
-- is kept with its portable identity instead of being dropped, so it can be
-- displayed and resolved lazily at play time. SQLite can't ALTER a column's
-- nullability/FK in place, so the table is recreated (works on Postgres too).
CREATE TABLE playlist_tracks_new (
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id    TEXT REFERENCES tracks(id) ON DELETE SET NULL,
    position    INTEGER NOT NULL,
    added_by    TEXT NOT NULL DEFAULT '',
    added_at    INTEGER NOT NULL,
    mbid        TEXT NOT NULL DEFAULT '',
    artist      TEXT NOT NULL DEFAULT '',
    title       TEXT NOT NULL DEFAULT '',
    album       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (playlist_id, position)
);
INSERT INTO playlist_tracks_new (playlist_id, track_id, position, added_by, added_at)
    SELECT playlist_id, track_id, position, added_by, added_at FROM playlist_tracks;
DROP TABLE playlist_tracks;
ALTER TABLE playlist_tracks_new RENAME TO playlist_tracks;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM playlist_tracks WHERE track_id IS NULL;
CREATE TABLE playlist_tracks_old (
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id    TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    added_by    TEXT NOT NULL DEFAULT '',
    added_at    INTEGER NOT NULL,
    PRIMARY KEY (playlist_id, position)
);
INSERT INTO playlist_tracks_old (playlist_id, track_id, position, added_by, added_at)
    SELECT playlist_id, track_id, position, added_by, added_at FROM playlist_tracks;
DROP TABLE playlist_tracks;
ALTER TABLE playlist_tracks_old RENAME TO playlist_tracks;
-- +goose StatementEnd
