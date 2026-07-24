// Package scanner walks library folders, extracts audio metadata and populates
// the catalog idempotently. It supports full scans, incremental file events and
// periodic rescans.
package scanner

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// Scanner indexes audio files into the catalog.
type Scanner struct {
	catalog   *persistence.CatalogRepo
	genres    *persistence.GenreRepo
	extractor *Extractor
	coversDir string
	logger    *slog.Logger

	mu       sync.Mutex
	scanning bool

	// onComplete, if set, runs after every full scan (initial, periodic or
	// manual). Used to refresh derived caches (library stats) and wake the artist
	// image enricher.
	onComplete func(context.Context, Result)
}

// Scanning reports whether a full scan is currently in progress.
func (s *Scanner) Scanning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scanning
}

// SetOnComplete registers a callback invoked after each full scan completes.
func (s *Scanner) SetOnComplete(fn func(context.Context, Result)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onComplete = fn
}

// Result summarizes a scan.
type Result struct {
	Scanned int
	Added   int
	Updated int
	Removed int
	Errors  int
	Elapsed time.Duration
}

// New builds a Scanner. coversDir receives extracted embedded cover art.
func New(catalog *persistence.CatalogRepo, genres *persistence.GenreRepo, extractor *Extractor, coversDir string, logger *slog.Logger) *Scanner {
	return &Scanner{
		catalog:   catalog,
		genres:    genres,
		extractor: extractor,
		coversDir: coversDir,
		logger:    logger,
	}
}

// ScanPaths performs a full scan of the given roots, pruning tracks whose files
// have disappeared. It is safe to call repeatedly; reruns do not create dupes.
func (s *Scanner) ScanPaths(ctx context.Context, paths []string) (Result, error) {
	s.mu.Lock()
	if s.scanning {
		s.mu.Unlock()
		return Result{}, nil
	}
	s.scanning = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.scanning = false
		s.mu.Unlock()
	}()

	start := time.Now()
	var res Result

	if err := os.MkdirAll(s.coversDir, 0o755); err != nil {
		return res, err
	}

	// Track existing local files to detect removals.
	existing, err := s.catalog.AllTrackPaths(ctx)
	if err != nil {
		return res, err
	}
	// Track the ids actually indexed this run. Pruning by id (not by path) is
	// essential so that a relocated/renamed file — which keeps its track id but
	// changes path — is not mistaken for a deleted file.
	seenIDs := make(map[string]bool, len(existing))

	for _, root := range paths {
		// Skip configured roots that don't exist yet (e.g. the on-demand download
		// directory before the first download) instead of logging a walk error.
		if _, statErr := os.Stat(root); statErr != nil {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				s.logger.Warn("walk error", "path", path, "error", walkErr)
				return nil
			}
			if d.IsDir() {
				return ctx.Err()
			}
			if _, ok := IsAudioFile(path); !ok {
				return nil
			}
			abs, _ := filepath.Abs(path)
			res.Scanned++
			id, added, err := s.indexFile(ctx, abs, existing)
			if err != nil {
				res.Errors++
				s.logger.Warn("index error", "path", abs, "error", err)
				// A file that failed to index (e.g. a transient ffprobe/read error)
				// is still present on disk — WalkDir just visited it — so keep its
				// existing track rather than letting the prune step below delete it
				// and its annotations. A genuinely-removed file is never visited.
				if prevID, ok := existing[abs]; ok {
					seenIDs[prevID] = true
				}
				return nil
			}
			seenIDs[id] = true
			if added {
				res.Added++
			} else {
				res.Updated++
			}
			return nil
		})
		if err != nil {
			return res, err
		}
	}

	// Prune tracks that were not re-indexed this run (their files disappeared).
	// DeleteTrackCascade, not the plain delete: a vanished-from-disk track is
	// exactly as gone as an explicitly deleted one, so its annotations (e.g. a
	// starred flag) must go with it — annotations has no DB-level FK to tracks
	// (item_id is polymorphic, shared with artists/albums), so a plain delete
	// would leave a dangling starred row that later crashes
	// autoplaylists' forgotten-favorites sync with a foreign key violation when
	// it tries to add that dead track id to a playlist.
	for _, id := range existing {
		if !seenIDs[id] {
			if err := s.catalog.DeleteTrackCascade(ctx, id); err != nil {
				s.logger.Warn("prune error", "track", id, "error", err)
				continue
			}
			res.Removed++
		}
	}

	res.Elapsed = time.Since(start)
	s.logger.Info("scan complete", "scanned", res.Scanned, "added", res.Added,
		"updated", res.Updated, "removed", res.Removed, "errors", res.Errors, "elapsed", res.Elapsed)
	s.mu.Lock()
	onComplete := s.onComplete
	s.mu.Unlock()
	if onComplete != nil {
		onComplete(ctx, res)
	}
	return res, nil
}

// ScanFile indexes a single file (used by the incremental watcher).
func (s *Scanner) ScanFile(ctx context.Context, path string) error {
	if _, ok := IsAudioFile(path); !ok {
		return nil
	}
	abs, _ := filepath.Abs(path)
	_, _, err := s.indexFile(ctx, abs, nil)
	return err
}

// IngestFile indexes a single file and returns the resulting track id. Used by
// the upload endpoint, which needs the id to mark ownership.
func (s *Scanner) IngestFile(ctx context.Context, path string) (string, error) {
	if _, ok := IsAudioFile(path); !ok {
		return "", fmt.Errorf("unsupported audio file: %s", filepath.Base(path))
	}
	abs, _ := filepath.Abs(path)
	id, _, err := s.indexFile(ctx, abs, nil)
	return id, err
}

// RemoveFile deletes the track for a removed file path. DeleteTrackCascade,
// same reasoning as the prune step in Scan: a removed file is gone the same
// way an explicit delete is, so its annotations must go with it too.
func (s *Scanner) RemoveFile(ctx context.Context, path string) error {
	abs, _ := filepath.Abs(path)
	existing, err := s.catalog.AllTrackPaths(ctx)
	if err != nil {
		return err
	}
	if id, ok := existing[abs]; ok {
		return s.catalog.DeleteTrackCascade(ctx, id)
	}
	return nil
}

// indexFile upserts the artist/album/track for a single file. It returns the
// resulting track id and whether the track was newly added. existingPaths, when
// non-nil, is the already-loaded path→id set used to detect an exact-path match
// without re-querying the catalog per file (a full scan passes it to avoid O(N²)
// lookups); single-file callers pass nil to have it queried on demand.
func (s *Scanner) indexFile(ctx context.Context, path string, existingPaths map[string]string) (string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, err
	}
	md, err := s.extractor.Extract(ctx, path)
	if err != nil {
		return "", false, err
	}
	hash, err := hashFile(path)
	if err != nil {
		return "", false, err
	}

	now := time.Now()

	// Resolve the artist. For compilations we still attribute the track to its
	// performing artist, but album artist drives album grouping.
	albumArtistName := firstNonEmpty(md.AlbumArtist, md.Artist)
	albumArtistID, err := s.catalog.UpsertArtist(ctx, models.Artist{
		ID:        uuid.NewString(),
		Name:      albumArtistName,
		SortName:  firstNonEmpty(md.AlbumArtistSort, md.ArtistSort, albumArtistName),
		MBID:      md.MBArtistID,
		CreatedAt: now,
	})
	if err != nil {
		return "", false, err
	}

	trackArtistID := albumArtistID
	if md.Artist != "" && md.Artist != albumArtistName {
		trackArtistID, err = s.catalog.UpsertArtist(ctx, models.Artist{
			ID:        uuid.NewString(),
			Name:      md.Artist,
			SortName:  firstNonEmpty(md.ArtistSort, md.Artist),
			CreatedAt: now,
		})
		if err != nil {
			return "", false, err
		}
	}

	albumID, err := s.catalog.UpsertAlbum(ctx, models.Album{
		ID:            uuid.NewString(),
		Name:          md.Album,
		SortName:      firstNonEmpty(md.AlbumSort, md.Album),
		ArtistID:      albumArtistID,
		MBID:          md.MBAlbumID,
		Year:          md.Year,
		Genre:         md.Genre,
		IsCompilation: md.Compilation,
		CreatedAt:     now,
	})
	if err != nil {
		return "", false, err
	}

	// Persist genre and cover art.
	if md.Genre != "" {
		if _, err := s.genres.Upsert(ctx, uuid.NewString(), md.Genre); err != nil {
			s.logger.Warn("genre upsert", "error", err)
		}
	}
	coverArt := ""
	if md.HasPicture && len(md.Picture) > 0 {
		if err := s.writeCover(albumID, md.Picture); err == nil {
			coverArt = albumID
		}
	} else if existing, err := s.catalog.GetAlbum(ctx, albumID); err == nil && existing.CoverArt != "" {
		coverArt = existing.CoverArt
	}

	suffix := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	ct, _ := IsAudioFile(path)

	// Determine if this is an add vs update before upserting.
	_, existed, _ := s.catalog.TrackExistsByMBIDOrHash(ctx, md.MBTrackID, hash)
	if !existed {
		// Also treat an exact-path match as existing.
		paths := existingPaths
		if paths == nil {
			paths, _ = s.catalog.AllTrackPaths(ctx)
		}
		if _, ok := paths[path]; ok {
			existed = true
		}
	}

	track := models.Track{
		ID:              uuid.NewString(),
		Title:           md.Title,
		AlbumID:         albumID,
		ArtistID:        trackArtistID,
		TrackNo:         md.TrackNo,
		DiscNo:          md.DiscNo,
		Composer:        md.Composer,
		Genre:           md.Genre,
		Year:            md.Year,
		BPM:             md.BPM,
		Duration:        md.Duration,
		BitRate:         md.BitRate,
		TitleSort:       md.TitleSort,
		Work:            md.Work,
		MovementName:    md.MovementName,
		MovementNo:      md.MovementNo,
		Lyrics:          md.Lyrics,
		Participants:    md.Participants,
		ReplayGainTrack: md.ReplayGainTrack,
		ReplayGainAlbum: md.ReplayGainAlbum,
		Path:            path,
		Suffix:          suffix,
		ContentType:     ct,
		Size:            info.Size(),
		MBID:            md.MBTrackID,
		ISRC:            md.ISRC,
		FileHash:        hash,
		CoverArt:        coverArt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	id, err := s.catalog.UpsertTrack(ctx, track)
	if err != nil {
		return "", false, err
	}
	return id, !existed, nil
}

func (s *Scanner) writeCover(albumID string, data []byte) error {
	if err := os.MkdirAll(s.coversDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.coversDir, albumID), data, 0o644)
}

// hashFile computes the md5 of the file contents (used for rename detection so
// that moving/renaming a file preserves its identity and annotations).
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
