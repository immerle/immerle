-- +goose Up
-- +goose StatementBegin
CREATE TABLE podcast_providers (
    name       TEXT PRIMARY KEY,
    enabled    INTEGER NOT NULL DEFAULT 0,
    config     TEXT NOT NULL DEFAULT '{}',
    updated_at INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE podcast_providers;
-- +goose StatementEnd
