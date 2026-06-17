package scanner

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher performs incremental scanning: it watches library roots with fsnotify
// and optionally triggers a periodic full rescan.
type Watcher struct {
	scanner *Scanner
	paths   []string
	// interval returns the periodic-rescan interval, read live so the admin can
	// change it at runtime (0 disables). May be nil.
	interval func() time.Duration
	logger   *slog.Logger
	// debounce collects rapid successive events per path before indexing.
	debounce time.Duration
}

// NewWatcher builds a Watcher. interval returns the periodic rescan cadence
// (read live; 0 or nil disables periodic rescans).
func NewWatcher(s *Scanner, paths []string, interval func() time.Duration, logger *slog.Logger) *Watcher {
	return &Watcher{
		scanner:  s,
		paths:    paths,
		interval: interval,
		logger:   logger,
		debounce: 2 * time.Second,
	}
}

// scanInterval reads the current interval (0 when disabled or unset).
func (w *Watcher) scanInterval() time.Duration {
	if w.interval == nil {
		return 0
	}
	return w.interval()
}

// Run starts watching until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() { _ = watcher.Close() }()

	for _, root := range w.paths {
		w.addRecursive(watcher, root)
	}

	// Debounce map of pending file events. A 1s tick drives both the debounce
	// flush and the periodic rescan, whose cadence is re-read each tick so a
	// runtime change to the scan interval takes effect without a restart.
	pending := make(map[string]time.Time)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	lastScan := time.Now()

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			w.handleEvent(watcher, event, pending)

		case <-tick.C:
			// Periodic full rescan (interval read live → hot-reloadable).
			if iv := w.scanInterval(); iv > 0 && time.Since(lastScan) >= iv {
				lastScan = time.Now()
				if _, err := w.scanner.ScanPaths(ctx, w.paths); err != nil {
					w.logger.Warn("periodic scan failed", "error", err)
				}
			}
			// Debounce flush of pending file events.
			now := time.Now()
			for path, t := range pending {
				if now.Sub(t) < w.debounce {
					continue
				}
				delete(pending, path)
				if _, err := os.Stat(path); err != nil {
					if err := w.scanner.RemoveFile(ctx, path); err != nil {
						w.logger.Warn("remove failed", "path", path, "error", err)
					}
					continue
				}
				if err := w.scanner.ScanFile(ctx, path); err != nil {
					w.logger.Warn("incremental index failed", "path", path, "error", err)
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			w.logger.Warn("watcher error", "error", err)
		}
	}
}

func (w *Watcher) handleEvent(watcher *fsnotify.Watcher, event fsnotify.Event, pending map[string]time.Time) {
	// Newly created directories must be watched too.
	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			w.addRecursive(watcher, event.Name)
			return
		}
	}
	if _, ok := IsAudioFile(event.Name); !ok {
		return
	}
	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename|fsnotify.Remove) != 0 {
		pending[event.Name] = time.Now()
	}
}

func (w *Watcher) addRecursive(watcher *fsnotify.Watcher, root string) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if addErr := watcher.Add(path); addErr != nil {
				w.logger.Warn("watch add failed", "path", path, "error", addErr)
			}
		}
		return nil
	})
}
