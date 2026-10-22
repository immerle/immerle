-- +goose Up
-- +goose StatementBegin
ALTER TABLE playlists ADD COLUMN cover_art TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE playlists DROP COLUMN cover_art;
-- +goose StatementEnd
