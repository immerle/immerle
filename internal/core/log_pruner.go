package core

import (
	"context"
	"log/slog"
	"time"

	"github.com/immerle/immerle/internal/persistence"
)

// LogPruner deletes old rows from the provider log table once per interval.
// The retention window is read live from the runtime settings so the admin can
// change it without a restart; a non-positive retention disables pruning.
type LogPruner struct {
	store     *persistence.ProviderLogRepo
	retention func() time.Duration
	interval  time.Duration
	logger    *slog.Logger
}

// NewLogPruner builds a LogPruner. interval is the sweep cadence (default 24h).
func NewLogPruner(retention func() time.Duration, interval time.Duration, logger *slog.Logger, store *persistence.ProviderLogRepo) *LogPruner {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &LogPruner{store: store, retention: retention, interval: interval, logger: logger}
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
	removed, err := p.store.PruneOlderThan(ctx, cutoff)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		p.logger.Warn("log prune failed", "table", p.store.LogTableName(), "error", err)
	} else if removed > 0 {
		p.logger.Info("pruned old logs", "table", p.store.LogTableName(), "removed", removed)
	}
}
