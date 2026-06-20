-- +goose Up
-- +goose StatementBegin
ALTER TABLE albums ADD COLUMN sort_name TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE tracks ADD COLUMN title_sort TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
-- TIT1 content group / classical work.
ALTER TABLE tracks ADD COLUMN work TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE tracks ADD COLUMN movement_name TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE tracks ADD COLUMN movement_no INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE tracks ADD COLUMN lyrics TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
-- JSON array of {role,name} contributors beyond the main artist.
ALTER TABLE tracks ADD COLUMN participants TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE tracks DROP COLUMN participants;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE tracks DROP COLUMN lyrics;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE tracks DROP COLUMN movement_no;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE tracks DROP COLUMN movement_name;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE tracks DROP COLUMN work;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE tracks DROP COLUMN title_sort;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE albums DROP COLUMN sort_name;
-- +goose StatementEnd
