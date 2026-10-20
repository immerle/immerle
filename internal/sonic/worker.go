package sonic

import (
	"context"
	"log/slog"
	"os/exec"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

const (
	batchSize = 32
	idle      = 2 * time.Minute
)

// Worker analyzes the library in the background, a batch at a time. It is
// always started and gates itself on the live settings, so toggling the
// feature needs no restart. It pauses while a library scan runs.
type Worker struct {
	repo      *persistence.SonicRepo
	ffmpeg    string
	available bool
	config    func() models.SonicRuntime
	busy      func() bool
	logger    *slog.Logger
	wake      chan struct{}
}

// NewWorker builds a worker. busy reports whether a scan is running; config
// returns the live settings.
func NewWorker(repo *persistence.SonicRepo, ffmpeg string, config func() models.SonicRuntime, busy func() bool, logger *slog.Logger) *Worker {
	_, err := exec.LookPath(ffmpeg)
	if err != nil {
		logger.Info("sonic analysis unavailable: ffmpeg not found", "ffmpeg", ffmpeg)
	}
	return &Worker{repo: repo, ffmpeg: ffmpeg, available: err == nil, config: config, busy: busy, logger: logger, wake: make(chan struct{}, 1)}
}

// Available reports whether ffmpeg was found, i.e. whether analysis can run at all.
func (w *Worker) Available() bool { return w != nil && w.available }

// Enabled reports whether analysis is both possible and switched on.
func (w *Worker) Enabled() bool { return w.Available() && w.config().Enabled }

// Wake makes an idle worker check for work now (after a scan, or once the
// feature is switched on).
func (w *Worker) Wake() {
	if w == nil {
		return
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Stats returns the analysis progress over the local library.
func (w *Worker) Stats(ctx context.Context) (persistence.SonicStats, error) {
	return w.repo.Stats(ctx, AlgoVersion)
}

// Run drains the backlog until ctx is done.
func (w *Worker) Run(ctx context.Context) {
	if !w.Available() {
		return
	}
	for {
		saved := 0
		if w.config().Enabled && !w.busy() {
			var err error
			if saved, err = w.runBatch(ctx); err != nil && ctx.Err() == nil {
				w.logger.Warn("sonic analysis batch failed", "error", err)
			}
		}
		if saved > 0 {
			continue // more backlog may remain
		}
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-time.After(idle):
		}
	}
}

// runBatch analyzes one batch of pending tracks, returning how many results
// were stored.
func (w *Worker) runBatch(ctx context.Context) (int, error) {
	tracks, err := w.repo.PendingTracks(ctx, AlgoVersion, batchSize)
	if err != nil || len(tracks) == 0 {
		return 0, err
	}
	var saved atomic.Int32
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(w.config().Workers)
	for _, t := range tracks {
		g.Go(func() error {
			res, ok := w.analyze(gctx, t)
			if !ok {
				return nil
			}
			if err := w.repo.Save(gctx, res); err != nil {
				w.logger.Warn("could not save sonic features", "track", t.ID, "error", err)
				return nil
			}
			saved.Add(1)
			return nil
		})
	}
	_ = g.Wait()
	w.logger.Info("sonic analysis batch done", "analyzed", saved.Load(), "batch", len(tracks))
	return int(saved.Load()), nil
}

// analyze decodes and analyzes one track. ok is false when the result must not
// be stored (shutdown); a failed analysis is stored with Error set so it isn't
// retried until the file or the algorithm changes.
func (w *Worker) analyze(ctx context.Context, t models.Track) (models.TrackSonic, bool) {
	res := models.TrackSonic{TrackID: t.ID, FileHash: t.FileHash, AlgoVersion: AlgoVersion}
	samples, err := Decode(ctx, w.ffmpeg, t.Path, t.Duration)
	if ctx.Err() != nil {
		return res, false
	}
	var f Features
	if err == nil {
		f, err = Analyze(samples)
	}
	res.AnalyzedAt = time.Now()
	if err != nil {
		res.Error = truncate(err.Error(), 300)
		w.logger.Debug("sonic analysis failed", "track", t.ID, "path", t.Path, "error", err)
		return res, true
	}
	res.BPM, res.Key, res.Mode, res.Camelot = f.BPM, f.Key, f.Mode, f.Camelot
	res.Energy, res.Loudness, res.Danceability, res.Brightness = f.Energy, f.Loudness, f.Danceability, f.Brightness
	res.Vector = f.Vector
	return res, true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
