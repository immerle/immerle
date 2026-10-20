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

// Service runs the background analysis of the library, a batch at a time, and
// answers similarity queries from the results. The worker is always started
// and gates itself on the live settings, so toggling the feature needs no
// restart. It pauses while a library scan runs.
type Service struct {
	repo      *persistence.SonicRepo
	ffmpeg    string
	available bool
	config    func() models.SonicRuntime
	busy      func() bool
	logger    *slog.Logger
	wake      chan struct{}
	index     *index
}

// NewService builds the service. busy reports whether a scan is running;
// config returns the live settings.
func NewService(repo *persistence.SonicRepo, ffmpeg string, config func() models.SonicRuntime, busy func() bool, logger *slog.Logger) *Service {
	_, err := exec.LookPath(ffmpeg)
	if err != nil {
		logger.Info("sonic analysis unavailable: ffmpeg not found", "ffmpeg", ffmpeg)
	}
	return &Service{
		repo: repo, ffmpeg: ffmpeg, available: err == nil, config: config, busy: busy, logger: logger,
		wake: make(chan struct{}, 1), index: newIndex(repo),
	}
}

// Available reports whether ffmpeg was found, i.e. whether analysis can run at all.
func (s *Service) Available() bool { return s != nil && s.available }

// Enabled reports whether analysis is both possible and switched on.
func (s *Service) Enabled() bool { return s.Available() && s.config().Enabled }

// RadioEnabled reports whether the sonic track radio is switched on.
func (s *Service) RadioEnabled() bool { return s.Enabled() && s.config().Radio }

// AutoplayEnabled reports whether endless queues (sonic autoplay) are switched on.
func (s *Service) AutoplayEnabled() bool { return s.Enabled() && s.config().Autoplay }

// Wake makes an idle worker check for work now (after a scan, or once the
// feature is switched on).
func (s *Service) Wake() {
	if s == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Stats returns the analysis progress over the local library.
func (s *Service) Stats(ctx context.Context) (persistence.SonicStats, error) {
	return s.repo.Stats(ctx, AlgoVersion)
}

// Similar returns up to count analyzed track ids that sound like the seeds,
// excluding the seeds and the exclude list. ok is false when none of the seeds
// is analyzed, so the caller can fall back to another strategy.
func (s *Service) Similar(ctx context.Context, seeds, exclude []string, count int) (ids []string, ok bool, err error) {
	return s.index.similar(ctx, seeds, exclude, count)
}

// Run drains the backlog until ctx is done.
func (s *Service) Run(ctx context.Context) {
	if !s.Available() {
		return
	}
	for {
		saved := 0
		if s.config().Enabled && !s.busy() {
			var err error
			if saved, err = s.runBatch(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("sonic analysis batch failed", "error", err)
			}
		}
		if saved > 0 {
			s.index.markDirty()
			continue // more backlog may remain
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-time.After(idle):
		}
	}
}

// runBatch analyzes one batch of pending tracks, returning how many results
// were stored.
func (s *Service) runBatch(ctx context.Context) (int, error) {
	tracks, err := s.repo.PendingTracks(ctx, AlgoVersion, batchSize)
	if err != nil || len(tracks) == 0 {
		return 0, err
	}
	var saved atomic.Int32
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.config().Workers)
	for _, t := range tracks {
		g.Go(func() error {
			res, ok := s.analyze(gctx, t)
			if !ok {
				return nil
			}
			if err := s.repo.Save(gctx, res); err != nil {
				s.logger.Warn("could not save sonic features", "track", t.ID, "error", err)
				return nil
			}
			saved.Add(1)
			return nil
		})
	}
	_ = g.Wait()
	s.logger.Info("sonic analysis batch done", "analyzed", saved.Load(), "batch", len(tracks))
	return int(saved.Load()), nil
}

// analyze decodes and analyzes one track. ok is false when the result must not
// be stored (shutdown); a failed analysis is stored with Error set so it isn't
// retried until the file or the algorithm changes.
func (s *Service) analyze(ctx context.Context, t models.Track) (models.TrackSonic, bool) {
	res := models.TrackSonic{TrackID: t.ID, FileHash: t.FileHash, AlgoVersion: AlgoVersion}
	samples, err := Decode(ctx, s.ffmpeg, t.Path, t.Duration)
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
		s.logger.Debug("sonic analysis failed", "track", t.ID, "path", t.Path, "error", err)
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
