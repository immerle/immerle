-- +goose Up
-- +goose StatementBegin
ALTER TABLE play_queues ADD COLUMN shuffle INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE play_queues ADD COLUMN repeat_mode TEXT NOT NULL DEFAULT 'off';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE play_queues DROP COLUMN shuffle;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE play_queues DROP COLUMN repeat_mode;
-- +goose StatementEnd
