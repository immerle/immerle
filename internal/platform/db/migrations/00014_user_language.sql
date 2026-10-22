-- +goose Up
ALTER TABLE users ADD COLUMN language TEXT NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE users DROP COLUMN language;
