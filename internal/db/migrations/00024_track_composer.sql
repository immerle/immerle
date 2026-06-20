-- +goose Up
-- +goose StatementBegin
-- Composer tag (ID3 TCOM / Vorbis COMPOSER). Empty when the file has no composer.
ALTER TABLE tracks ADD COLUMN composer TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE tracks DROP COLUMN composer;
-- +goose StatementEnd
