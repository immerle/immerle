package persistence

import (
	"context"
	"time"

	melody "github.com/ermos/melody/v2"
	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// ProviderLogRepo persists warn/error events from provider actions so the admin
// can inspect per-provider failures.
type ProviderLogRepo struct{ *base }

// Insert records a provider event. Best-effort: callers ignore the error.
func (r *ProviderLogRepo) Insert(ctx context.Context, l models.ProviderLog) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("provider_logs").
		Set("id", uuid.NewString()).Set("provider", l.Provider).Set("level", l.Level).
		Set("action", l.Action).Set("message", l.Message).Set("created_at", db.Millis(time.Now())))
	return err
	// ponytail: table grows unbounded; add a retention sweep if it ever matters.
}

// LogTableName identifies this log store to the daily pruner.
func (r *ProviderLogRepo) LogTableName() string { return "provider_logs" }

// PruneOlderThan deletes log rows created before cutoff and returns the number
// removed. Used by the daily core.LogPruner.
func (r *ProviderLogRepo) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.bexec(ctx, r.mel.NewDelete("provider_logs").Where("created_at", "<", db.Millis(cutoff)))
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
	rows, err := r.bquery(ctx, r.mel.New("provider_logs").
		Select("id", "provider", "level", "action", "message", "created_at").
		Where("provider", "=", provider).
		OrderBy("created_at", melody.Desc).OrderBy("id", melody.Desc).Limit(limit))
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
