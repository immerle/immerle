-- +goose Up
-- +goose StatementBegin
ALTER TABLE api_tokens ADD COLUMN is_device INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE api_tokens DROP COLUMN is_device;
-- +goose StatementEnd
