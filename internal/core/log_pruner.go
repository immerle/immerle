package core

import (
	"context"
	"log/slog"
	"time"
)

// PrunableLog is any persisted log table that can drop rows older than a cutoff.
// Implement it on a repo (see persistence.ProviderLogRepo) to have the daily
// LogPruner sweep it — that is the single extension point for future log types.
type PrunableLog interface {
	PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	LogTableName() string
}

// LogPruner deletes old rows from the registered log tables once per interval.
// The retention window is read live from the runtime settings so the admin can
// change it without a restart; a non-positive retention disables pruning.
type LogPruner struct {
	stores    []PrunableLog
	retention func() time.Duration
	interval  time.Duration
	logger    *slog.Logger
}

// NewLogPruner builds a LogPruner. interval is the sweep cadence (default 24h).
func NewLogPruner(retention func() time.Duration, interval time.Duration, logger *slog.Logger, stores ...PrunableLog) *LogPruner {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &LogPruner{stores: stores, retention: retention, interval: interval, logger: logger}
}

// Run sweeps on the configured interval until ctx is cancelled. It sweeps once
// at startup, then every interval. A non-positive retention skips the tick.
func (p *LogPruner) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		p.sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *LogPruner) sweep(ctx context.Context) {
	retention := p.retention()
	if retention <= 0 {
		return // keep forever
	}
	cutoff := time.Now().Add(-retention)
	for _, s := range p.stores {
		removed, err := s.PruneOlderThan(ctx, cutoff)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.logger.Warn("log prune failed", "table", s.LogTableName(), "error", err)
		} else if removed > 0 {
			p.logger.Info("pruned old logs", "table", s.LogTableName(), "removed", removed)
		}
	}
}
