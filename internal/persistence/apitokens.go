package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	melody "github.com/ermos/melody/v2"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// APITokenRepo persists personal API tokens.
type APITokenRepo struct{ *base }

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
	return t, err
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
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListDeviceSessions returns the user's device-kind tokens last used at or
// after `since` — the pool of playback-transfer targets ("connected
// devices"), excluding manually-created personal/CLI tokens.
func (r *APITokenRepo) ListDeviceSessions(ctx context.Context, userID string, since time.Time) ([]models.APIToken, error) {
	rows, err := r.bquery(ctx, r.mel.New("api_tokens").Select(apiTokenColumns).
		Where("user_id", "=", userID).Where("revoked", "=", 0).Where("is_device", "=", 1).
		Where("last_used_at", ">=", db.Millis(since)).OrderBy("last_used_at", melody.Desc))
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
		out = append(out, t)
	}
	return out, rows.Err()
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

// TouchLastUsed records the last-used time (best effort).
func (r *APITokenRepo) TouchLastUsed(ctx context.Context, id string, at time.Time) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("api_tokens").Set("last_used_at", db.Millis(at)).Where("id", "=", id))
	return err
}
