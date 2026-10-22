-- +goose Up
-- +goose StatementBegin
CREATE TABLE provider_logs (
    id         TEXT PRIMARY KEY,
    provider   TEXT NOT NULL,
    level      TEXT NOT NULL,
    action     TEXT NOT NULL,
    message    TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_provider_logs_provider ON provider_logs (provider, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS provider_logs;
-- +goose StatementEnd
