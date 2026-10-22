-- +goose Up
-- +goose StatementBegin
ALTER TABLE play_queues ADD COLUMN command_json TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE play_queues ADD COLUMN command_seq INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE play_queues DROP COLUMN command_json;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE play_queues DROP COLUMN command_seq;
-- +goose StatementEnd
