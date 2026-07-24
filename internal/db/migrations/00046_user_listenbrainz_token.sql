-- +goose Up
ALTER TABLE users ADD COLUMN listenbrainz_token TEXT NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE users DROP COLUMN listenbrainz_token;
