-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN theme TEXT NOT NULL DEFAULT '{}';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN theme;
-- +goose StatementEnd
