-- +goose Up
-- +goose StatementBegin
-- Owner of a user-uploaded ("local") track. Empty for scanned/provider tracks.
ALTER TABLE tracks ADD COLUMN uploaded_by TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_tracks_uploaded_by ON tracks(uploaded_by) WHERE uploaded_by <> '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_tracks_uploaded_by;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE tracks DROP COLUMN uploaded_by;
-- +goose StatementEnd
