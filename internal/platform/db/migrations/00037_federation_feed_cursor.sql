-- +goose Up
-- +goose StatementBegin
-- Per-source-instance replay cursor for the federation feed socket (see
-- RFC-socket-federation-client.md §4): on reconnect, the client resumes from
-- last_version instead of re-pulling the whole feed.
CREATE TABLE federation_feed_cursor (
    source_instance_id TEXT PRIMARY KEY,
    last_version        TEXT NOT NULL DEFAULT '',
    updated_at           INTEGER NOT NULL DEFAULT 0
);

-- Extend playlist_sync with the resolved payload (covers already rewritten to
-- hub URLs) and its version, so a replay.request (§6) can be answered without
-- recomputing anything.
ALTER TABLE playlist_sync ADD COLUMN last_payload TEXT NOT NULL DEFAULT '{}';
ALTER TABLE playlist_sync ADD COLUMN last_version TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE playlist_sync DROP COLUMN last_version;
ALTER TABLE playlist_sync DROP COLUMN last_payload;
DROP TABLE federation_feed_cursor;
-- +goose StatementEnd
