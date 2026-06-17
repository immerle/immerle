-- +goose Up
-- +goose StatementBegin
ALTER TABLE import_items ADD COLUMN candidate_cover_art TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE import_items DROP COLUMN candidate_cover_art;
-- +goose StatementEnd
