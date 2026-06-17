-- +goose Up
-- +goose StatementBegin
ALTER TABLE artists ADD COLUMN image_checked INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE artists DROP COLUMN image_checked;
-- +goose StatementEnd
