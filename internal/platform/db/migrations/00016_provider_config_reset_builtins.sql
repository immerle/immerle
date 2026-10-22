-- Breaking change: provider config moves to the unified { header, params, … }
-- schema. Drop the seeded built-in rows so they are re-seeded at boot with the
-- new default config and order (ensureBuiltins). Custom (http) providers keep
-- their rows; the config parser accepts the legacy "headers" alias.
-- +goose Up
-- +goose StatementBegin
DELETE FROM provider_configs WHERE kind = 'builtin';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Irreversible: built-ins are re-seeded from code on the next boot.
-- +goose StatementEnd
