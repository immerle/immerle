-- +goose Up
-- +goose StatementBegin
ALTER TABLE tracks ADD COLUMN mb_checked INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE tracks DROP COLUMN mb_checked;
-- +goose StatementEnd
