package core

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/bandcamp"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/scanner"
)

// Ingester turns a downloaded audio file on disk into a catalog track,
// reading its embedded tags. Satisfied by *scanner.Scanner.
type Ingester interface {
	IngestFile(ctx context.Context, path string) (string, error)
}

const (
	// bandcampMaxFileBytes matches uploads.go's maxUploadBytes — one purchased
	// file shouldn't need to be larger than a manually uploaded one.
	bandcampMaxFileBytes = 200 << 20
	// bandcampMaxArchiveBytes caps a whole album zip download.
	bandcampMaxArchiveBytes = 2 << 30
	bandcampCollectionPage  = 50
)

// PurchasesService connects a user's personal Bandcamp account (there's no
// official OAuth, so the user pastes their browser session cookie) and
// imports their purchased albums/tracks into their library.
type PurchasesService struct {
	conns      *persistence.BandcampConnectionRepo
	jobs       *persistence.BandcampImportRepo
	bc         *bandcamp.Client
	catalog    *persistence.CatalogRepo
	ingester   Ingester
	box        *secretBox
	uploadsDir string
	logger     *slog.Logger
	wakeCh     chan struct{}
}

// NewPurchasesService builds a PurchasesService. secret is the instance config
// secret — like AuthService, it's used to derive the key that encrypts stored
// cookies at rest.
func NewPurchasesService(conns *persistence.BandcampConnectionRepo, jobs *persistence.BandcampImportRepo,
	bc *bandcamp.Client, catalog *persistence.CatalogRepo, ingester Ingester,
	uploadsDir, secret string, logger *slog.Logger) (*PurchasesService, error) {
	box, err := newSecretBox(secret)
	if err != nil {
		return nil, err
	}
	return &PurchasesService{
		conns: conns, jobs: jobs, bc: bc, catalog: catalog, ingester: ingester,
		box: box, uploadsDir: uploadsDir, logger: logger, wakeCh: make(chan struct{}, 1),
	}, nil
}

// Connect validates cookie against Bandcamp and stores it (encrypted),
// replacing any previous connection. Returns the resolved fan id.
func (s *PurchasesService) Connect(ctx context.Context, userID, cookie string) (string, error) {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return "", errors.New("cookie is required")
	}
	fanID, err := s.bc.FanID(ctx, cookie)
	if err != nil {
		return "", err
	}
	enc, err := s.box.Encrypt(cookie)
	if err != nil {
		return "", err
	}
	if err := s.conns.Upsert(ctx, models.BandcampConnection{
		UserID: userID, FanID: fanID, IdentityEnc: enc, ConnectedAt: time.Now(),
	}); err != nil {
		return "", err
	}
	return fanID, nil
}

// Disconnect removes a user's Bandcamp connection.
func (s *PurchasesService) Disconnect(ctx context.Context, userID string) error {
	return s.conns.Delete(ctx, userID)
}

// Status returns a user's connection, and connected=false (no error) if they
// haven't connected one.
func (s *PurchasesService) Status(ctx context.Context, userID string) (models.BandcampConnection, bool, error) {
	c, err := s.conns.Get(ctx, userID)
	if errors.Is(err, persistence.ErrNotFound) {
		return models.BandcampConnection{}, false, nil
	}
	if err != nil {
		return models.BandcampConnection{}, false, err
	}
	return c, true, nil
}

// ListCollection fetches a user's full Bandcamp purchase collection, live.
func (s *PurchasesService) ListCollection(ctx context.Context, userID string) ([]bandcamp.CollectionItem, error) {
	conn, err := s.conns.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	cookie, err := s.box.Decrypt(conn.IdentityEnc)
	if err != nil {
		return nil, err
	}
	items, err := s.collectAll(ctx, cookie, conn.FanID)
	if err != nil {
		if errors.Is(err, bandcamp.ErrInvalidCookie) {
			_ = s.conns.MarkInvalid(ctx, userID, time.Now())
		}
		return nil, err
	}
	_ = s.conns.TouchSynced(ctx, userID, time.Now())
	return items, nil
}

// collectAll pages through a fan's entire collection.
func (s *PurchasesService) collectAll(ctx context.Context, cookie, fanID string) ([]bandcamp.CollectionItem, error) {
	var out []bandcamp.CollectionItem
	token := ""
	for {
		page, err := s.bc.Collection(ctx, cookie, fanID, token, bandcampCollectionPage)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		if !page.MoreAvailable || page.LastToken == "" {
			break
		}
		token = page.LastToken
	}
	return out, nil
}

// findItem re-pages the collection to find one item. Needed at job-claim time
// since Bandcamp has no single-item lookup and a redownload URL captured
// earlier may no longer be valid.
func (s *PurchasesService) findItem(ctx context.Context, cookie, fanID, saleItemType, saleItemID string) (bandcamp.CollectionItem, error) {
	token := ""
	for {
		page, err := s.bc.Collection(ctx, cookie, fanID, token, bandcampCollectionPage)
		if err != nil {
			return bandcamp.CollectionItem{}, err
		}
		for _, it := range page.Items {
			if it.SaleItemType == saleItemType && it.SaleItemID == saleItemID {
				return it, nil
			}
		}
		if !page.MoreAvailable || page.LastToken == "" {
			break
		}
		token = page.LastToken
	}
	return bandcamp.CollectionItem{}, fmt.Errorf("bandcamp: item %s%s not found in collection", saleItemType, saleItemID)
}

// EnqueueImport queues a purchased item for download+ingest. Idempotent:
// re-calling for an item that's already queued/imported returns the existing job.
func (s *PurchasesService) EnqueueImport(ctx context.Context, userID string, item bandcamp.CollectionItem) (models.BandcampImportJob, error) {
	now := time.Now()
	job, _, err := s.jobs.Enqueue(ctx, models.BandcampImportJob{
		ID: uuid.NewString(), UserID: userID,
		SaleItemType: item.SaleItemType, SaleItemID: item.SaleItemID, ItemType: item.ItemType,
		ArtistName: item.ArtistName, ItemTitle: item.ItemTitle,
		Status: models.BandcampQueued, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return models.BandcampImportJob{}, err
	}
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
	return job, nil
}

// ListJobs returns a user's import jobs, most recent first.
func (s *PurchasesService) ListJobs(ctx context.Context, userID string) ([]models.BandcampImportJob, error) {
	return s.jobs.ListByUser(ctx, userID, 100)
}

// Worker runs the background import queue until ctx is cancelled, resuming
// jobs left 'running' by a previous crash. Mirrors CatalogService.Worker.
func (s *PurchasesService) Worker(ctx context.Context) {
	_ = s.jobs.RequeueStale(ctx)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		s.drainQueue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-s.wakeCh:
		case <-ticker.C:
		}
	}
}

func (s *PurchasesService) drainQueue(ctx context.Context) {
	var lastID string
	repeats := 0
	for {
		job, err := s.jobs.ClaimNext(ctx)
		if err != nil {
			return // empty queue or error
		}
		// Guard against a job that stays eligible and is re-claimed without ever
		// making progress: stop draining so the outer ticker retries later
		// instead of tight-looping.
		if job.ID == lastID {
			if repeats++; repeats >= 3 {
				return
			}
		} else {
			lastID, repeats = job.ID, 0
		}
		trackIDs, err := s.processJob(ctx, job)
		if err != nil {
			if errors.Is(err, bandcamp.ErrInvalidCookie) {
				_ = s.conns.MarkInvalid(ctx, job.UserID, time.Now())
				_ = s.jobs.Fail(ctx, job.ID, err.Error(), false)
			} else {
				s.logger.Error("bandcamp import failed", "job", job.ID, "err", err)
				_ = s.jobs.Fail(ctx, job.ID, err.Error(), job.Attempts < 3)
			}
			continue
		}
		_ = s.jobs.Complete(ctx, job.ID, trackIDs)
	}
}

func (s *PurchasesService) processJob(ctx context.Context, job models.BandcampImportJob) ([]string, error) {
	conn, err := s.conns.Get(ctx, job.UserID)
	if err != nil {
		return nil, fmt.Errorf("not connected: %w", err)
	}
	cookie, err := s.box.Decrypt(conn.IdentityEnc)
	if err != nil {
		return nil, err
	}
	item, err := s.findItem(ctx, cookie, conn.FanID, job.SaleItemType, job.SaleItemID)
	if err != nil {
		return nil, err
	}
	if item.RedownloadURL == "" {
		return nil, errors.New("bandcamp: no redownload url for this item")
	}
	info, err := s.bc.ResolveDownload(ctx, cookie, item.RedownloadURL)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(s.uploadsDir, job.UserID, "Bandcamp", sanitize(job.ArtistName), sanitize(job.ItemTitle))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(destDir, "download-*.tmp")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	maxBytes := int64(bandcampMaxFileBytes)
	if job.ItemType == "album" {
		maxBytes = bandcampMaxArchiveBytes
	}
	dlErr := s.bc.Download(ctx, cookie, info.URL, tmp, maxBytes)
	closeErr := tmp.Close()
	if dlErr != nil {
		return nil, dlErr
	}
	if closeErr != nil {
		return nil, closeErr
	}

	var files []string
	if job.ItemType == "album" {
		files, err = extractZip(tmpPath, destDir, bandcampMaxFileBytes)
	} else {
		dest := filepath.Join(destDir, sanitize(job.ItemTitle)+formatExtension(info.Format))
		if err = os.Rename(tmpPath, dest); err == nil {
			files = []string{dest}
		}
	}
	if err != nil {
		return nil, err
	}

	var trackIDs []string
	for _, f := range files {
		trackID, err := s.ingester.IngestFile(ctx, f)
		if err != nil {
			s.logger.Warn("bandcamp: failed to ingest extracted file", "file", f, "err", err)
			continue
		}
		if err := s.catalog.SetTrackOwner(ctx, trackID, job.UserID); err != nil {
			return nil, err
		}
		trackIDs = append(trackIDs, trackID)
	}
	if len(trackIDs) == 0 {
		return nil, errors.New("bandcamp: no audio files found in download")
	}
	return trackIDs, nil
}

// extractZip extracts every audio entry from archivePath into destDir,
// flattening paths (Bandcamp album zips are flat) and rejecting anything that
// would resolve outside destDir (zip-slip), capping each entry's decompressed
// size at maxFileBytes.
func extractZip(archivePath, destDir string, maxFileBytes int64) ([]string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()

	var out []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if _, ok := scanner.IsAudioFile(f.Name); !ok {
			continue
		}
		destPath := filepath.Join(destDir, filepath.Base(filepath.Clean(f.Name)))
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("bandcamp: zip entry %q escapes destination directory", f.Name)
		}
		if err := extractZipEntry(f, destPath, maxFileBytes); err != nil {
			return nil, err
		}
		out = append(out, destPath)
	}
	return out, nil
}

func extractZipEntry(f *zip.File, destPath string, maxBytes int64) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	n, err := io.Copy(out, io.LimitReader(rc, maxBytes+1))
	if err != nil {
		return err
	}
	if n > maxBytes {
		return fmt.Errorf("bandcamp: zip entry %q exceeds size limit", f.Name)
	}
	return nil
}

var bandcampFormatExtensions = map[string]string{
	"flac": ".flac", "mp3-320": ".mp3", "mp3-v0": ".mp3", "aac-hi": ".m4a",
	"alac": ".m4a", "vorbis": ".ogg", "wav": ".wav", "aiff-lossless": ".aiff",
}

func formatExtension(format string) string {
	return bandcampFormatExtensions[format]
}
