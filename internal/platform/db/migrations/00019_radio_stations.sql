-- +goose Up
-- +goose StatementBegin
CREATE TABLE radio_stations (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    stream_url   TEXT NOT NULL,
    homepage_url TEXT NOT NULL DEFAULT '',
    builtin      INTEGER NOT NULL DEFAULT 0,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE radio_stations;
-- +goose StatementEnd
