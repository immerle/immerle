package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/testutil"
)

// tinyPNG is a minimal valid PNG (1x1) so looksLikeImage accepts it.
var tinyPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d,
	'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89,
	0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

// fakeImageLookup is a stub ArtistImageLookup returning a fixed URL.
type fakeImageLookup struct {
	available bool
	url       string
	err       error
}

func (f fakeImageLookup) Available() bool { return f.available }
func (f fakeImageLookup) Lookup(_ context.Context, _ string) (string, error) {
	return f.url, f.err
}

func TestArtistImageEnricherFetchesAndCaches(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	var imgHits int
	images := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imgHits++
		_, _ = w.Write(tinyPNG)
	}))
	defer images.Close()

	// Seed two artists needing images.
	id1, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Artist One", CreatedAt: time.Now()})
	_, _ = store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Artist Two", CreatedAt: time.Now()})

	coversDir := filepath.Join(t.TempDir(), "covers")
	lookup := fakeImageLookup{available: true, url: images.URL + "/dp.png"}
	enr := NewArtistImageEnricher(store.Catalog, lookup, coversDir, 0, testutil.NewLogger())

	processed, fetched, err := enr.EnrichMissing(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 2 || fetched != 2 {
		t.Fatalf("expected processed=2 fetched=2, got %d/%d", processed, fetched)
	}

	// Avatar cached on disk and cover_art set on the artist.
	if _, err := os.Stat(filepath.Join(coversDir, id1)); err != nil {
		t.Fatalf("avatar not cached for artist 1: %v", err)
	}
	a, _ := store.Catalog.GetArtist(ctx, id1)
	if a.CoverArt != id1 {
		t.Fatalf("artist cover_art not set, got %q", a.CoverArt)
	}

	// Re-running finds nothing to do (image_checked flips off the backlog).
	processed, _, _ = enr.EnrichMissing(ctx, 10)
	if processed != 0 {
		t.Fatalf("expected no remaining candidates, got %d", processed)
	}
}

func TestEnricherRejectsNonImage(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>not an image</html>"))
	}))
	defer html.Close()

	id, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Bad", CreatedAt: time.Now()})
	lookup := fakeImageLookup{available: true, url: html.URL + "/x"}
	enr := NewArtistImageEnricher(store.Catalog, lookup, filepath.Join(t.TempDir(), "c"), 0, testutil.NewLogger())

	_, fetched, _ := enr.EnrichMissing(ctx, 10)
	if fetched != 0 {
		t.Fatal("non-image response must not be cached as an avatar")
	}
	// Still marked checked so it won't be retried forever.
	a, _ := store.Catalog.GetArtist(ctx, id)
	if a.CoverArt != "" {
		t.Fatal("cover_art should remain empty for a rejected image")
	}
	processed, _, _ := enr.EnrichMissing(ctx, 10)
	if processed != 0 {
		t.Fatal("rejected artist should be marked checked (not retried)")
	}
}

func TestEnricherNoProviderIsNoop(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	_, _ = store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Lonely", CreatedAt: time.Now()})

	enr := NewArtistImageEnricher(store.Catalog, fakeImageLookup{available: false}, filepath.Join(t.TempDir(), "c"), 0, testutil.NewLogger())
	processed, fetched, err := enr.EnrichMissing(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 || fetched != 0 {
		t.Fatalf("expected no work when no provider supplies images, got %d/%d", processed, fetched)
	}
}
