-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN display_name;
-- +goose StatementEnd
