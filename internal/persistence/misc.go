package persistence

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// ---- Play queues ----

// PlayQueueRepo persists per-user server-side play queues.
type PlayQueueRepo struct{ *base }

// Save stores the user's queue.
func (r *PlayQueueRepo) Save(ctx context.Context, q models.PlayQueue) error {
	ids := strings.Join(q.TrackIDs, ",")
	_, err := r.exec(ctx, `INSERT INTO play_queues (user_id, track_ids, current, position_ms, changed_by, changed_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET track_ids=excluded.track_ids, current=excluded.current,
		position_ms=excluded.position_ms, changed_by=excluded.changed_by, changed_at=excluded.changed_at`,
		q.UserID, ids, q.Current, q.PositionMs, q.ChangedBy, db.Millis(q.ChangedAt))
	return err
}

// Get returns the user's queue, or ErrNotFound.
func (r *PlayQueueRepo) Get(ctx context.Context, userID string) (models.PlayQueue, error) {
	var q models.PlayQueue
	var ids string
	var changedAt int64
	err := r.queryRow(ctx, `SELECT user_id, track_ids, current, position_ms, changed_by, changed_at
		FROM play_queues WHERE user_id=?`, userID).Scan(&q.UserID, &ids, &q.Current, &q.PositionMs, &q.ChangedBy, &changedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return q, ErrNotFound
	}
	if err != nil {
		return q, err
	}
	if ids != "" {
		q.TrackIDs = strings.Split(ids, ",")
	}
	q.ChangedAt = db.FromMillis(changedAt)
	return q, nil
}

// ---- Scrobbles ----

// ScrobbleRepo persists scrobble submissions.
type ScrobbleRepo struct{ *base }

// Insert records a scrobble.
func (r *ScrobbleRepo) Insert(ctx context.Context, s models.Scrobble) error {
	_, err := r.exec(ctx, `INSERT INTO scrobbles (id, user_id, track_id, played_at, submitted, exported)
		VALUES (?, ?, ?, ?, ?, ?)`, s.ID, s.UserID, s.TrackID, db.Millis(s.PlayedAt), db.Bool(s.Submitted), db.Bool(s.Exported))
	return err
}

// Unexported returns scrobbles not yet pushed to the hub.
func (r *ScrobbleRepo) Unexported(ctx context.Context, limit int) ([]models.Scrobble, error) {
	rows, err := r.query(ctx, `SELECT id, user_id, track_id, played_at, submitted, exported
		FROM scrobbles WHERE exported=0 AND submitted=1 ORDER BY played_at LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Scrobble
	for rows.Next() {
		var s models.Scrobble
		var playedAt int64
		var submitted, exported int
		if err := rows.Scan(&s.ID, &s.UserID, &s.TrackID, &playedAt, &submitted, &exported); err != nil {
			return nil, err
		}
		s.PlayedAt = db.FromMillis(playedAt)
		s.Submitted = submitted != 0
		s.Exported = exported != 0
		out = append(out, s)
	}
	return out, rows.Err()
}

// MarkExported flags scrobbles as exported to the hub.
func (r *ScrobbleRepo) MarkExported(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if _, err := r.exec(ctx, `UPDATE scrobbles SET exported=1 WHERE id=?`, id); err != nil {
			return err
		}
	}
	return nil
}

// ---- Shares ----

// ShareRepo persists public share links.
type ShareRepo struct{ *base }

// Create inserts a share.
func (r *ShareRepo) Create(ctx context.Context, s models.Share) error {
	_, err := r.exec(ctx, `INSERT INTO shares (id, user_id, item_type, item_id, secret, description, expires_at, created_at, view_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		s.ID, s.UserID, string(s.ItemType), s.ItemID, s.Secret, s.Description, db.NullMillis(s.ExpiresAt), db.Millis(s.CreatedAt))
	return err
}

// Delete removes a share owned by user.
func (r *ShareRepo) Delete(ctx context.Context, id, userID string) error {
	_, err := r.exec(ctx, `DELETE FROM shares WHERE id=? AND user_id=?`, id, userID)
	return err
}

func scanShare(s interface{ Scan(...any) error }) (models.Share, error) {
	var sh models.Share
	var itemType string
	var expires sql.NullInt64
	var created int64
	if err := s.Scan(&sh.ID, &sh.UserID, &itemType, &sh.ItemID, &sh.Secret, &sh.Description, &expires, &created, &sh.ViewCount); err != nil {
		return sh, err
	}
	sh.ItemType = models.ItemType(itemType)
	sh.ExpiresAt = db.TimePtr(expires)
	sh.CreatedAt = db.FromMillis(created)
	return sh, nil
}

const shareColumns = `id, user_id, item_type, item_id, secret, description, expires_at, created_at, view_count`

// Get returns a share by id.
func (r *ShareRepo) Get(ctx context.Context, id string) (models.Share, error) {
	row := r.queryRow(ctx, `SELECT `+shareColumns+` FROM shares WHERE id=?`, id)
	sh, err := scanShare(row)
	if errors.Is(err, sql.ErrNoRows) {
		return sh, ErrNotFound
	}
	return sh, err
}

// GetBySecret returns a share by its public secret.
func (r *ShareRepo) GetBySecret(ctx context.Context, secret string) (models.Share, error) {
	row := r.queryRow(ctx, `SELECT `+shareColumns+` FROM shares WHERE secret=?`, secret)
	sh, err := scanShare(row)
	if errors.Is(err, sql.ErrNoRows) {
		return sh, ErrNotFound
	}
	return sh, err
}

// ListByUser returns a user's shares.
func (r *ShareRepo) ListByUser(ctx context.Context, userID string) ([]models.Share, error) {
	rows, err := r.query(ctx, `SELECT `+shareColumns+` FROM shares WHERE user_id=? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Share
	for rows.Next() {
		sh, err := scanShare(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// IncrementViews bumps a share's view counter.
func (r *ShareRepo) IncrementViews(ctx context.Context, id string) error {
	_, err := r.exec(ctx, `UPDATE shares SET view_count = view_count + 1 WHERE id=?`, id)
	return err
}

// Update changes a share's description and expiry (owner-scoped).
func (r *ShareRepo) Update(ctx context.Context, id, userID, description string, expiresAt *time.Time) error {
	_, err := r.exec(ctx, `UPDATE shares SET description=?, expires_at=? WHERE id=? AND user_id=?`,
		description, db.NullMillis(expiresAt), id, userID)
	return err
}

// ---- Provider cache ----

// ProviderCacheRepo caches provider responses.
type ProviderCacheRepo struct{ *base }

// Get returns a cached response, or ErrNotFound.
func (r *ProviderCacheRepo) Get(ctx context.Context, provider, queryHash string) ([]byte, error) {
	var resp []byte
	err := r.queryRow(ctx, `SELECT response FROM provider_cache WHERE provider=? AND query_hash=?`, provider, queryHash).Scan(&resp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return resp, err
}

// Put stores a cached response.
func (r *ProviderCacheRepo) Put(ctx context.Context, id, provider, queryHash string, response []byte) error {
	_, err := r.exec(ctx, `INSERT INTO provider_cache (id, provider, query_hash, response, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(provider, query_hash) DO UPDATE SET response=excluded.response, created_at=excluded.created_at`,
		id, provider, queryHash, response, db.Millis(time.Now()))
	return err
}
