-- +goose Up
-- +goose StatementBegin
CREATE TABLE playlist_subscriptions (
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  INTEGER NOT NULL,
    PRIMARY KEY (playlist_id, user_id)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_playlist_subscriptions_user ON playlist_subscriptions(user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS playlist_subscriptions;
-- +goose StatementEnd
