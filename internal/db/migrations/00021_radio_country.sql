-- +goose Up
-- +goose StatementBegin
ALTER TABLE radio_stations ADD COLUMN country TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE radio_stations DROP COLUMN country;
-- +goose StatementEnd
