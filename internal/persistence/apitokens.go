package persistence

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"sync"
	"time"

	melody "github.com/ermos/melody/v2"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// APITokenRepo persists personal API tokens.
type APITokenRepo struct {
	*base
	// seenMu/pending buffer TouchLastUsed writes in memory instead of hitting
	// the DB on every token-authenticated request (nearly every request, once
	// a client has exchanged its login for a token — see auth.go); FlushLastUsed
	// persists them in one batch. Reads overlay pending entries so they still
	// see the freshest state. Mirrors DeviceRepo's TouchSeen/FlushSeen.
	seenMu  sync.Mutex
	pending map[string]models.APIToken
}

const apiTokenColumns = `id, user_id, name, token_hash, prefix, created_at, last_used_at, expires_at, revoked, is_device`

func scanAPIToken(s rowScanner) (models.APIToken, error) {
	var t models.APIToken
	var created int64
	var lastUsed, expires sql.NullInt64
	var revoked, isDevice int
	if err := s.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.Prefix, &created, &lastUsed, &expires, &revoked, &isDevice); err != nil {
		return t, err
	}
	t.CreatedAt = db.FromMillis(created)
	t.LastUsedAt = db.TimePtr(lastUsed)
	t.ExpiresAt = db.TimePtr(expires)
	t.Revoked = revoked != 0
	t.IsDevice = isDevice != 0
	return t, nil
}

// Create inserts a token.
func (r *APITokenRepo) Create(ctx context.Context, t models.APIToken) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("api_tokens").
		Set("id", t.ID).Set("user_id", t.UserID).Set("name", t.Name).Set("token_hash", t.TokenHash).
		Set("prefix", t.Prefix).Set("created_at", db.Millis(t.CreatedAt)).
		Set("last_used_at", db.NullMillis(t.LastUsedAt)).Set("expires_at", db.NullMillis(t.ExpiresAt)).
		Set("revoked", db.Bool(t.Revoked)).Set("is_device", db.Bool(t.IsDevice)))
	return err
}

// GetByHash returns a non-revoked token by its hash.
func (r *APITokenRepo) GetByHash(ctx context.Context, hash string) (models.APIToken, error) {
	row := r.bqueryRow(ctx, r.mel.New("api_tokens").Select(apiTokenColumns).
		Where("token_hash", "=", hash).Where("revoked", "=", 0))
	t, err := scanAPIToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	return r.withPending(t), nil
}

// ListByUser returns a user's tokens, most recent first.
func (r *APITokenRepo) ListByUser(ctx context.Context, userID string) ([]models.APIToken, error) {
	rows, err := r.bquery(ctx, r.mel.New("api_tokens").Select(apiTokenColumns).
		Where("user_id", "=", userID).Where("revoked", "=", 0).OrderBy("created_at", melody.Desc))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.APIToken
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r.withPending(t))
	}
	return out, rows.Err()
}

// ListDeviceSessions returns the user's device-kind tokens last used at or
// after `since` — the pool of playback-transfer targets ("connected
// devices"), excluding manually-created personal/CLI tokens. The since filter
// is applied in Go, after overlaying pending touches, so a token active only
// in the not-yet-flushed buffer isn't wrongly excluded by a SQL WHERE clause
// that can only see the last-flushed DB value.
func (r *APITokenRepo) ListDeviceSessions(ctx context.Context, userID string, since time.Time) ([]models.APIToken, error) {
	rows, err := r.bquery(ctx, r.mel.New("api_tokens").Select(apiTokenColumns).
		Where("user_id", "=", userID).Where("revoked", "=", 0).Where("is_device", "=", 1))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.APIToken
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		t = r.withPending(t)
		if t.LastUsedAt != nil && !t.LastUsedAt.Before(since) {
			out = append(out, t)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsedAt.After(*out[j].LastUsedAt) })
	return out, nil
}

// Revoke marks a token revoked (owner-scoped). Returns whether a row matched.
func (r *APITokenRepo) Revoke(ctx context.Context, id, userID string) (bool, error) {
	res, err := r.bexec(ctx, r.mel.NewUpdate("api_tokens").Set("revoked", 1).
		Where("id", "=", id).Where("user_id", "=", userID))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// TouchLastUsed buffers a token's last-used time in memory rather than
// writing to the DB on every authenticated request. tok is the caller's
// current view of the token (already fetched via GetByHash), so a flush can
// recreate its row from this snapshot if it's ever missing. Call
// FlushLastUsed (on shutdown) to persist buffered touches.
func (r *APITokenRepo) TouchLastUsed(_ context.Context, tok models.APIToken, at time.Time) error {
	tok.LastUsedAt = &at
	r.seenMu.Lock()
	if r.pending == nil {
		r.pending = make(map[string]models.APIToken)
	}
	r.pending[tok.ID] = tok
	r.seenMu.Unlock()
	return nil
}

// FlushLastUsed persists every buffered last-used touch to the DB. A hard
// kill between flushes only loses those touches — same trade-off as
// DeviceRepo.FlushSeen.
func (r *APITokenRepo) FlushLastUsed(ctx context.Context) error {
	r.seenMu.Lock()
	pending := r.pending
	r.pending = nil
	r.seenMu.Unlock()

	for _, tok := range pending {
		res, err := r.bexec(ctx, r.mel.NewUpdate("api_tokens").
			Set("last_used_at", db.NullMillis(tok.LastUsedAt)).Where("id", "=", tok.ID))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// The row disappeared between the original GetByHash and this
			// flush (or never existed) — recreate it from the cached snapshot.
			if err := r.Create(ctx, tok); err != nil {
				return err
			}
		}
	}
	return nil
}

// withPending overlays a not-yet-flushed touch onto a DB-scanned row, so reads
// reflect the freshest last-used state even before the next FlushLastUsed.
func (r *APITokenRepo) withPending(t models.APIToken) models.APIToken {
	r.seenMu.Lock()
	p, ok := r.pending[t.ID]
	r.seenMu.Unlock()
	if ok {
		t.LastUsedAt = p.LastUsedAt
	}
	return t
}
