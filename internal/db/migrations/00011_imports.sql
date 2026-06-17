-- +goose Up
-- +goose StatementBegin
CREATE TABLE imports (
    id                   TEXT PRIMARY KEY,
    user_id              TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source               TEXT NOT NULL,
    source_ref           TEXT NOT NULL,
    source_playlist_name TEXT NOT NULL DEFAULT '',
    playlist_id          TEXT REFERENCES playlists(id) ON DELETE SET NULL,
    status               TEXT NOT NULL,
    total                INTEGER NOT NULL DEFAULT 0,
    matched              INTEGER NOT NULL DEFAULT 0,
    doubtful             INTEGER NOT NULL DEFAULT 0,
    missing              INTEGER NOT NULL DEFAULT 0,
    failed               INTEGER NOT NULL DEFAULT 0,
    error                TEXT NOT NULL DEFAULT '',
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_imports_user ON imports(user_id, created_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE import_items (
    id               TEXT PRIMARY KEY,
    import_id        TEXT NOT NULL REFERENCES imports(id) ON DELETE CASCADE,
    position         INTEGER NOT NULL,
    source_title     TEXT NOT NULL DEFAULT '',
    source_artist    TEXT NOT NULL DEFAULT '',
    source_album     TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL,
    matched_track_id TEXT NOT NULL DEFAULT '',
    resolved_title   TEXT NOT NULL DEFAULT '',
    resolved_artist  TEXT NOT NULL DEFAULT '',
    confidence       REAL NOT NULL DEFAULT 0,
    note             TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_import_items_import ON import_items(import_id, position);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE import_items;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE imports;
-- +goose StatementEnd
