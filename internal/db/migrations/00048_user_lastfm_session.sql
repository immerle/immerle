-- +goose Up
ALTER TABLE users ADD COLUMN lastfm_session_key TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN lastfm_username TEXT NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE users DROP COLUMN lastfm_session_key;
ALTER TABLE users DROP COLUMN lastfm_username;
