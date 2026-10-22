-- +goose Up
-- +goose StatementBegin
ALTER TABLE provider_configs ADD COLUMN disabled_capabilities TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE provider_configs DROP COLUMN disabled_capabilities;
-- +goose StatementEnd
