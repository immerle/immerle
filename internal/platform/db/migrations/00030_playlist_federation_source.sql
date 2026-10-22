-- +goose Up
-- +goose StatementBegin
-- Origin of a federated playlist on the hub: source_instance_id is empty for
-- the hub's own editorial/recommendation catalog, else the owning instance's
-- hub id (subscription feed). The pair is the dedupe key, replacing the old
-- dedupe-by-name (which collapsed same-named playlists from different
-- instances into one). Backfill: existing federated rows were all editorial
-- (the feed never worked), so source_instance_id stays '' and
-- source_external_id takes the name they were already deduped by.
ALTER TABLE playlists ADD COLUMN source_instance_id TEXT NOT NULL DEFAULT '';
ALTER TABLE playlists ADD COLUMN source_external_id TEXT NOT NULL DEFAULT '';
UPDATE playlists SET source_external_id = name WHERE federated = 1;
CREATE UNIQUE INDEX idx_playlists_federated_source ON playlists (source_instance_id, source_external_id) WHERE federated = 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_playlists_federated_source;
ALTER TABLE playlists DROP COLUMN source_external_id;
ALTER TABLE playlists DROP COLUMN source_instance_id;
-- +goose StatementEnd
