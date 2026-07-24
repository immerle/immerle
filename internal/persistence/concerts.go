package persistence

import (
	"context"
	"database/sql"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// ConcertRepo persists per-user concert-discovery matches (see migration
// 00044). One row per (user, source, source event) — internal/concerts is the
// only writer.
type ConcertRepo struct{ *base }

func scanConcert(s rowScanner) (models.Concert, error) {
	var c models.Concert
	var startTime, createdAt int64
	var dismissedAt sql.NullInt64
	if err := s.Scan(&c.ID, &c.UserID, &c.Source, &c.SourceEventID, &c.ArtistName, &c.EventName,
		&c.Venue, &c.City, &startTime, &c.URL, &dismissedAt, &createdAt); err != nil {
		return c, err
	}
	c.StartTime = db.FromMillis(startTime)
	c.DismissedAt = db.TimePtr(dismissedAt)
	c.CreatedAt = db.FromMillis(createdAt)
	return c, nil
}

// Upsert inserts a newly-found match, reporting whether it was actually new.
// If a row already exists for this (userID, source, sourceEventID) — a prior
// sync already found it, dismissed or not — it is left completely untouched:
// a dismissal must never come back just because the daily sync ran again.
func (r *ConcertRepo) Upsert(ctx context.Context, c models.Concert) (bool, error) {
	res, err := r.exec(ctx, `INSERT INTO concerts
		(id, user_id, source, source_event_id, artist_name, event_name, venue, city, start_time, url, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, source, source_event_id) DO NOTHING`,
		c.ID, c.UserID, c.Source, c.SourceEventID, c.ArtistName, c.EventName, c.Venue, c.City,
		db.Millis(c.StartTime), c.URL, db.Millis(time.Now()))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ListActive returns a user's upcoming, non-dismissed concert matches,
// soonest first, capped at limit.
func (r *ConcertRepo) ListActive(ctx context.Context, userID string, now time.Time, limit int) ([]models.Concert, error) {
	rows, err := r.query(ctx, `SELECT id, user_id, source, source_event_id, artist_name, event_name, venue, city, start_time, url, dismissed_at, created_at
		FROM concerts
		WHERE user_id=? AND dismissed_at IS NULL AND start_time>=?
		ORDER BY start_time ASC
		LIMIT ?`, userID, db.Millis(now), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Concert
	for rows.Next() {
		c, err := scanConcert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Dismiss marks a concert dismissed for its owner (scoped by userID so a user
// can't dismiss another user's row). Reports whether a row was actually
// changed.
func (r *ConcertRepo) Dismiss(ctx context.Context, userID, id string) (bool, error) {
	res, err := r.exec(ctx, `UPDATE concerts SET dismissed_at=? WHERE id=? AND user_id=? AND dismissed_at IS NULL`,
		db.Millis(time.Now()), id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
