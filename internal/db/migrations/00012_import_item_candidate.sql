-- +goose Up
-- +goose StatementBegin
ALTER TABLE import_items ADD COLUMN candidate_id TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE import_items DROP COLUMN candidate_id;
-- +goose StatementEnd
