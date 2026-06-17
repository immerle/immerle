-- +goose Up
-- +goose StatementBegin
ALTER TABLE provider_configs ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE provider_configs DROP COLUMN sort_order;
-- +goose StatementEnd
