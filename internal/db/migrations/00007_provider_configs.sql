-- +goose Up
-- +goose StatementBegin
CREATE TABLE provider_configs (
    name       TEXT PRIMARY KEY,
    kind       TEXT NOT NULL DEFAULT 'http',
    endpoint   TEXT NOT NULL DEFAULT '',
    config     TEXT NOT NULL DEFAULT '{}',
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS provider_configs;
-- +goose StatementEnd
