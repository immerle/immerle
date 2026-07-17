-- +goose Up
-- +goose StatementBegin
ALTER TABLE tracks ADD COLUMN isrc TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE tracks DROP COLUMN isrc;
-- +goose StatementEnd
