-- +goose Up
-- +goose StatementBegin
ALTER TABLE play_queues ADD COLUMN playing INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE play_queues DROP COLUMN playing;
-- +goose StatementEnd
