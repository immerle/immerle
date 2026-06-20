-- +goose Up
-- +goose StatementBegin
DROP TABLE IF EXISTS provider_cache;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS provider_cache (
    id         TEXT PRIMARY KEY,
    provider   TEXT NOT NULL,
    query_hash TEXT NOT NULL,
    response   BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (provider, query_hash)
);
-- +goose StatementEnd
