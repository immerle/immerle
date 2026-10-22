-- +goose Up
-- +goose StatementBegin
ALTER TABLE play_queues ADD COLUMN entries_json TEXT NOT NULL DEFAULT '[]';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE play_queues DROP COLUMN entries_json;
-- +goose StatementEnd
