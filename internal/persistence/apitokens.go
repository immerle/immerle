package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// APITokenRepo persists personal API tokens.
type APITokenRepo struct{ *base }

const apiTokenColumns = `id, user_id, name, token_hash, prefix, created_at, last_used_at, expires_at, revoked`

func scanAPIToken(s interface{ Scan(...any) error }) (models.APIToken, error) {
	var t models.APIToken
	var created int64
	var lastUsed, expires sql.NullInt64
	var revoked int
	if err := s.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.Prefix, &created, &lastUsed, &expires, &revoked); err != nil {
		return t, err
	}
	t.CreatedAt = db.FromMillis(created)
	t.LastUsedAt = db.TimePtr(lastUsed)
	t.ExpiresAt = db.TimePtr(expires)
	t.Revoked = revoked != 0
	return t, nil
}

// Create inserts a token.
func (r *APITokenRepo) Create(ctx context.Context, t models.APIToken) error {
	_, err := r.exec(ctx, `INSERT INTO api_tokens (`+apiTokenColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.UserID, t.Name, t.TokenHash, t.Prefix, db.Millis(t.CreatedAt),
		db.NullMillis(t.LastUsedAt), db.NullMillis(t.ExpiresAt), db.Bool(t.Revoked))
	return err
}

// GetByHash returns a non-revoked token by its hash.
func (r *APITokenRepo) GetByHash(ctx context.Context, hash string) (models.APIToken, error) {
	row := r.queryRow(ctx, `SELECT `+apiTokenColumns+` FROM api_tokens WHERE token_hash=? AND revoked=0`, hash)
	t, err := scanAPIToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// ListByUser returns a user's tokens, most recent first.
func (r *APITokenRepo) ListByUser(ctx context.Context, userID string) ([]models.APIToken, error) {
	rows, err := r.query(ctx, `SELECT `+apiTokenColumns+` FROM api_tokens WHERE user_id=? AND revoked=0 ORDER BY created_at DESC`, userID)
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
	res, err := r.exec(ctx, `UPDATE api_tokens SET revoked=1 WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// TouchLastUsed records the last-used time (best effort).
func (r *APITokenRepo) TouchLastUsed(ctx context.Context, id string, at time.Time) error {
	_, err := r.exec(ctx, `UPDATE api_tokens SET last_used_at=? WHERE id=?`, db.Millis(at), id)
	return err
}
