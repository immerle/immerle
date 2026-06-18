package persistence

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// ProviderLogRepo persists warn/error events from provider actions so the admin
// can inspect per-provider failures.
type ProviderLogRepo struct{ *base }

// Insert records a provider event. Best-effort: callers ignore the error.
func (r *ProviderLogRepo) Insert(ctx context.Context, l models.ProviderLog) error {
	_, err := r.exec(ctx, `INSERT INTO provider_logs (id, provider, level, action, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), l.Provider, l.Level, l.Action, l.Message, db.Millis(time.Now()))
	return err
	// ponytail: table grows unbounded; add a retention sweep if it ever matters.
}

// LogTableName identifies this log store to the daily pruner.
func (r *ProviderLogRepo) LogTableName() string { return "provider_logs" }

// PruneOlderThan deletes log rows created before cutoff and returns the number
// removed. Implements the pruner's PrunableLog interface.
func (r *ProviderLogRepo) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.exec(ctx, `DELETE FROM provider_logs WHERE created_at < ?`, db.Millis(cutoff))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ListByProvider returns the most recent events for a provider, newest first.
func (r *ProviderLogRepo) ListByProvider(ctx context.Context, provider string, limit int) ([]models.ProviderLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.query(ctx, `SELECT id, provider, level, action, message, created_at
		FROM provider_logs WHERE provider=? ORDER BY created_at DESC, id DESC LIMIT ?`, provider, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ProviderLog
	for rows.Next() {
		var l models.ProviderLog
		var created int64
		if err := rows.Scan(&l.ID, &l.Provider, &l.Level, &l.Action, &l.Message, &created); err != nil {
			return nil, err
		}
		l.CreatedAt = db.FromMillis(created)
		out = append(out, l)
	}
	return out, rows.Err()
}
