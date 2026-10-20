-- +goose Up
-- +goose StatementBegin
-- Audio features computed locally by internal/sonic (one row per analyzed
-- track). file_hash and algo_version record what was analyzed, so a replaced
-- file or an improved algorithm gets re-analyzed. A failed analysis keeps a row
-- with error set, so it isn't retried until one of those changes.
CREATE TABLE IF NOT EXISTS track_sonic (
    track_id      TEXT PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
    file_hash     TEXT NOT NULL DEFAULT '',
    algo_version  BIGINT NOT NULL,
    bpm           REAL NOT NULL DEFAULT 0,
    musical_key   TEXT NOT NULL DEFAULT '',
    mode          TEXT NOT NULL DEFAULT '',
    camelot       TEXT NOT NULL DEFAULT '',
    energy        REAL NOT NULL DEFAULT 0,
    loudness      REAL NOT NULL DEFAULT 0,
    danceability  REAL NOT NULL DEFAULT 0,
    brightness    REAL NOT NULL DEFAULT 0,
    vector        BYTEA,
    error         TEXT NOT NULL DEFAULT '',
    analyzed_at   BIGINT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS track_sonic;
-- +goose StatementEnd
