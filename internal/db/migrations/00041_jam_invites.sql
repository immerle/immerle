-- +goose Up
-- +goose StatementBegin
-- Jam invites: a host inviting a specific user to their session. One pending
-- invite per (session, invitee) — re-inviting just refreshes created_at so it
-- resurfaces for an invitee who had dismissed it.
CREATE TABLE jam_invites (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES jam_sessions(id) ON DELETE CASCADE,
    inviter_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invitee_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    UNIQUE (session_id, invitee_id)
);
CREATE INDEX idx_jam_invites_invitee ON jam_invites (invitee_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE jam_invites;
-- +goose StatementEnd
