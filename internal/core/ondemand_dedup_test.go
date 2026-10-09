package core

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/providers"
	"github.com/immerle/immerle/internal/testutil"
)

// A provider track without an MBID (e.g. Deezer) must still be deduplicated out
// of remote search once it has been downloaded — via the completed download job,
// since the MBID dedup is a no-op for it.
func TestRemoteSearchDedupByCompletedDownload(t *testing.T) {
	store := testutil.NewStore(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	registry := NewProviderRegistry()
	registry.Register(&fakeProvider{name: "p", results: []providers.Result{
		{ProviderTrackID: "x1", Title: "No MBID Song", Artist: "A"}, // note: no MBID
	}})
	svc := NewCatalogService(CatalogServiceConfig{
		Catalog: store.Catalog, Downloads: store.Downloads, Registry: registry,
		Settings: StaticProviderSettings{}, Logger: logger,
	})

	// Before download → surfaced as a remote result.
	if r, _ := svc.RemoteSearch(ctx, "No MBID", 10); len(r) != 1 {
		t.Fatalf("expected 1 remote result before download, got %d", len(r))
	}

	// Simulate a completed download of that exact provider track.
	now := time.Now()
	job, _ := store.Downloads.Enqueue(ctx, models.DownloadJob{
		ID: uuid.NewString(), Provider: "p", ProviderTrackID: "x1",
		Status: models.DownloadQueued, CreatedAt: now, UpdatedAt: now,
	})
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A"})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID})
	trackID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "No MBID Song", ArtistID: artistID, AlbumID: albumID, Path: "/tmp/x.mp3",
	})
	if err := store.Downloads.Complete(ctx, job.ID, trackID); err != nil {
		t.Fatal(err)
	}

	// After download → deduped away (it's now the local track instead).
	if r, _ := svc.RemoteSearch(ctx, "No MBID", 10); len(r) != 0 {
		t.Fatalf("expected the downloaded track to be deduped from remote search, got %d", len(r))
	}
}

func TestLocalTrackIDForRemote(t *testing.T) {
	store := testutil.NewStore(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()
	svc := NewCatalogService(CatalogServiceConfig{
		Catalog: store.Catalog, Downloads: store.Downloads, Registry: NewProviderRegistry(), Logger: logger,
	})

	remoteID := "remote:p:x1"

	// No download yet → not mapped.
	if _, ok := svc.LocalTrackIDForRemote(ctx, remoteID); ok {
		t.Fatal("undownloaded remote track must not map to a local id")
	}
	// A non-remote id is never mapped.
	if _, ok := svc.LocalTrackIDForRemote(ctx, "local-1"); ok {
		t.Fatal("local id must not be treated as remote")
	}

	// Complete a download for (p, x1) → local-1.
	now := time.Now()
	job, _ := store.Downloads.Enqueue(ctx, models.DownloadJob{
		ID: uuid.NewString(), Provider: "p", ProviderTrackID: "x1",
		Status: models.DownloadQueued, CreatedAt: now, UpdatedAt: now,
	})
	if err := store.Downloads.Complete(ctx, job.ID, "local-1"); err != nil {
		t.Fatal(err)
	}

	if id, ok := svc.LocalTrackIDForRemote(ctx, remoteID); !ok || id != "local-1" {
		t.Fatalf("expected mapping to local-1, got %q ok=%v", id, ok)
	}
}

func TestSplitArtistsAndMatchKey(t *testing.T) {
	main, feats := splitArtists("Rick Ross/Drake/Chrisette Michele")
	if main != "Rick Ross" || len(feats) != 2 || feats[1] != "Chrisette Michele" {
		t.Fatalf("got %q %v", main, feats)
	}
	for _, p := range [][2]string{{"TOTO", "Toto"}, {"RŮDE", "Rude."}, {"JAY Z", "JAŸ-Z"}} {
		if matchKey(p[0]) != matchKey(p[1]) {
			t.Fatalf("%q and %q should match", p[0], p[1])
		}
	}
}

func TestPrepareStreamReusesOnDemandDuplicate(t *testing.T) {
	store := testutil.NewStore(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	registry := NewProviderRegistry()
	registry.Register(&fakeProvider{name: "p", results: []providers.Result{
		{ProviderTrackID: "new", Title: "africa", Artist: "TOTO/Someone", Album: "TOTO IV"},
		{ProviderTrackID: "scanned", Title: "Rosanna", Artist: "Toto", Album: "Toto IV"},
	}})
	svc := NewCatalogService(CatalogServiceConfig{
		Catalog: store.Catalog, Downloads: store.Downloads, Registry: registry, Logger: logger,
	})

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Toto"})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Toto IV", ArtistID: artistID})
	downloaded, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Africa", ArtistID: artistID, AlbumID: albumID, Path: "/tmp/africa.mp3",
	})
	_, _ = store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Rosanna", ArtistID: artistID, AlbumID: albumID, Path: "/tmp/rosanna.mp3",
	})
	now := time.Now()
	job, _ := store.Downloads.Enqueue(ctx, models.DownloadJob{
		ID: uuid.NewString(), Provider: "p", ProviderTrackID: "old",
		Status: models.DownloadQueued, CreatedAt: now, UpdatedAt: now,
	})
	_ = store.Downloads.Complete(ctx, job.ID, downloaded)

	tr, local, _, err := svc.PrepareStream(ctx, "", encodeRemoteID("p", "new"))
	if err != nil || !local || tr.ID != downloaded {
		t.Fatalf("expected the downloaded duplicate %s, got %s local=%v err=%v", downloaded, tr.ID, local, err)
	}
	if id, ok := svc.LocalTrackIDForRemote(ctx, encodeRemoteID("p", "new")); !ok || id != downloaded {
		t.Fatalf("the new provider track should now be linked to %s, got %q", downloaded, id)
	}

	// A manually scanned track (no download job) is never treated as a duplicate.
	_, local, pd, err := svc.PrepareStream(ctx, "", encodeRemoteID("p", "scanned"))
	if err != nil || local || pd == nil {
		t.Fatalf("scanned track must not dedup an on-demand download, local=%v err=%v", local, err)
	}
	if pd.Meta.Artist != "Toto" {
		t.Fatalf("artist should reuse the library spelling, got %q", pd.Meta.Artist)
	}
}
