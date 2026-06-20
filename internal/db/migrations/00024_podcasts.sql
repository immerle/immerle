-- +goose Up
-- +goose StatementBegin
CREATE TABLE podcast_channels (
    id          TEXT PRIMARY KEY,
    url         TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    cover_art   TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'new',
    error       TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);
CREATE TABLE podcast_episodes (
    id           TEXT PRIMARY KEY,
    channel_id   TEXT NOT NULL,
    guid         TEXT NOT NULL DEFAULT '',
    title        TEXT NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    publish_date INTEGER NOT NULL DEFAULT 0,
    duration     INTEGER NOT NULL DEFAULT 0,
    size         INTEGER NOT NULL DEFAULT 0,
    suffix       TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT '',
    bit_rate     INTEGER NOT NULL DEFAULT 0,
    stream_url   TEXT NOT NULL DEFAULT '',
    media_path   TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'skipped',
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    UNIQUE (channel_id, guid)
);
CREATE INDEX idx_podcast_episodes_channel ON podcast_episodes (channel_id, publish_date DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE podcast_episodes;
DROP TABLE podcast_channels;
-- +goose StatementEnd
