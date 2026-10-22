-- +goose Up
-- +goose StatementBegin
-- The friend/friendship feature is removed: on a self-hosted instance, users
-- already know each other, so a friends-only visibility tier added no real
-- gate. Existing "friends"-tier data becomes public (the closest surviving
-- tier) rather than disappearing silently.
UPDATE users SET activity_privacy = 'public' WHERE activity_privacy = 'friends';
UPDATE activity_events SET privacy = 'public' WHERE privacy = 'friends';
DROP TABLE IF EXISTS friendships;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE TABLE friendships (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    friend_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (user_id, friend_id)
);
-- +goose StatementEnd
