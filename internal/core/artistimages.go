package core

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/persistence"
)

// ArtistImageLookup resolves an artist's avatar URL. Artist images come from the
// on-demand provider (the same place artists themselves come from): if a
// registered provider exposes the capability, avatars are fetched through it.
type ArtistImageLookup interface {
	// Available reports whether a provider can currently supply artist images.
	Available() bool
	// Lookup returns a candidate image URL for an artist, or "" if none found.
	Lookup(ctx context.Context, artistName string) (string, error)
}

// ArtistImageEnricher fills in artist avatars by querying the provider image
// lookup, downloading the candidate image, validating it really is an image,
// caching it under coversDir/<artistID>, and pointing the artist's cover art at
// it. Failures mark the artist as checked so the loop does not retry forever.
type ArtistImageEnricher struct {
	catalog   *persistence.CatalogRepo
	lookup    ArtistImageLookup
	coversDir string
	http      *http.Client
	delay     time.Duration
	logger    *slog.Logger
	wake      chan struct{}
}

// NewArtistImageEnricher builds an enricher. delay throttles per-artist work.
func NewArtistImageEnricher(catalog *persistence.CatalogRepo, lookup ArtistImageLookup, coversDir string, delay time.Duration, logger *slog.Logger) *ArtistImageEnricher {
	return &ArtistImageEnricher{
		catalog:   catalog,
		lookup:    lookup,
		coversDir: coversDir,
		http:      &http.Client{Timeout: 30 * time.Second},
		delay:     delay,
		logger:    logger,
		wake:      make(chan struct{}, 1),
	}
}

// Wake nudges the background Run loop to re-check for artists needing avatars
// (e.g. right after a scan adds new artists), instead of waiting for the idle tick.
func (e *ArtistImageEnricher) Wake() {
	if e == nil {
		return
	}
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// EnrichMissing processes up to limit artists lacking an avatar. It returns the
// number of candidates processed and the number of avatars fetched. When no
// provider can supply images, it is a no-op (so it idles instead of spinning).
func (e *ArtistImageEnricher) EnrichMissing(ctx context.Context, limit int) (processed, fetched int, err error) {
	if e.lookup == nil || !e.lookup.Available() {
		return 0, 0, nil
	}
	artists, err := e.catalog.ListArtistsNeedingImage(ctx, limit)
	if err != nil {
		return 0, 0, err
	}
	for _, a := range artists {
		if err := ctx.Err(); err != nil {
			return processed, fetched, err
		}
		processed++
		if e.enrichOne(ctx, a.ID, a.Name) {
			fetched++
		}
		if e.delay > 0 {
			select {
			case <-ctx.Done():
				return processed, fetched, ctx.Err()
			case <-time.After(e.delay):
			}
		}
	}
	return processed, fetched, nil
}

// Run continuously enriches artists missing avatars: it drains the backlog in
// batches, then idles until new artists appear (e.g. after a scan).
func (e *ArtistImageEnricher) Run(ctx context.Context, idle time.Duration) {
	if idle <= 0 {
		idle = 30 * time.Minute
	}
	for {
		processed, fetched, err := e.EnrichMissing(ctx, 50)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.logger.Warn("artist image enrichment error", "error", err)
		}
		if processed > 0 {
			e.logger.Info("artist avatars enriched", "processed", processed, "fetched", fetched)
			continue // more backlog may remain; keep draining
		}
		select {
		case <-ctx.Done():
			return
		case <-e.wake:
		case <-time.After(idle):
		}
	}
}

func (e *ArtistImageEnricher) enrichOne(ctx context.Context, id, name string) bool {
	imgURL, err := e.lookup.Lookup(ctx, name)
	if err != nil {
		e.logger.Debug("artist image lookup failed", "artist", name, "error", err)
		return false // transient — retry next round, don't mark as permanently checked
	}
	if imgURL != "" {
		data, derr := e.download(ctx, imgURL)
		if derr == nil && strings.HasPrefix(http.DetectContentType(data), "image/") {
			if err := e.save(id, data); err != nil {
				e.logger.Warn("could not cache artist image", "artist", name, "error", err)
			} else if err := e.catalog.SetArtistCover(ctx, id, id); err != nil {
				e.logger.Warn("could not set artist cover", "artist", name, "error", err)
			} else {
				e.logger.Debug("fetched artist avatar", "artist", name)
				return true
			}
		}
	}
	// Provider had no image for this artist — don't retry it next time.
	_ = e.catalog.MarkArtistImageChecked(ctx, id)
	return false
}

func (e *ArtistImageEnricher) download(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("image download status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 12<<20)) // cap at 12 MiB
}

func (e *ArtistImageEnricher) save(id string, data []byte) error {
	if err := os.MkdirAll(e.coversDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(e.coversDir, id), data, 0o644)
}
