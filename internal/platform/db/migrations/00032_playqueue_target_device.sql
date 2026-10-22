-- +goose Up
-- +goose StatementBegin
ALTER TABLE play_queues ADD COLUMN target_device_id TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE play_queues ADD COLUMN target_changed_at INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE play_queues DROP COLUMN target_device_id;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE play_queues DROP COLUMN target_changed_at;
-- +goose StatementEnd
