-- +goose Up
-- +goose StatementBegin
ALTER TABLE radio_stations ADD COLUMN cover_art TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE radio_stations DROP COLUMN cover_art;
-- +goose StatementEnd
