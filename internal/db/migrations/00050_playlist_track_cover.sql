-- +goose Up
-- Cover art of a federated playlist entry not resolved to a local track yet
-- (e.g. a Spotify playlist's album cover), so it shows before first play.
ALTER TABLE playlist_tracks ADD COLUMN cover_art TEXT NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE playlist_tracks DROP COLUMN cover_art;
