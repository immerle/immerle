-- +goose Up
-- +goose StatementBegin
CREATE TABLE smart_playlists (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    rules      TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_smart_playlists_owner ON smart_playlists(owner_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_smart_playlists_owner;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE smart_playlists;
-- +goose StatementEnd
