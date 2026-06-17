package core

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/persistence"
	"github.com/gossignol/gossignol/internal/providers"
	"github.com/gossignol/gossignol/internal/scanner"
	"github.com/gossignol/gossignol/internal/testutil"
)

// fakeProvider is a minimal in-memory provider for merge tests.
type fakeProvider struct {
	name    string
	results []providers.Result
}

func (f *fakeProvider) Name() string       { return f.name }
func (f *fakeProvider) MaxQuality() string { return "fake" }
func (f *fakeProvider) Search(_ context.Context, _ string, _ int) ([]providers.Result, error) {
	return f.results, nil
}
func (f *fakeProvider) Resolve(_ context.Context, id string) (providers.Result, error) {
	for _, r := range f.results {
		if r.ProviderTrackID == id {
			return r, nil
		}
	}
	return providers.Result{}, io.EOF
}
func (f *fakeProvider) Download(_ context.Context, _ string, _ io.Writer) error { return nil }

// fileProvider is a test provider that serves a single local audio file — the
// downloadable counterpart of fakeProvider, used to exercise the full on-demand
// download pipeline now that the shipped sample provider is gone.
type fileProvider struct {
	name string
	path string
	res  providers.Result
}

func (p *fileProvider) Name() string       { return p.name }
func (p *fileProvider) MaxQuality() string { return "test" }
func (p *fileProvider) Search(_ context.Context, query string, _ int) ([]providers.Result, error) {
	if strings.Contains(strings.ToLower(p.res.Title), strings.ToLower(query)) {
		return []providers.Result{p.res}, nil
	}
	return nil, nil
}
func (p *fileProvider) Resolve(_ context.Context, id string) (providers.Result, error) {
	if id == p.res.ProviderTrackID {
		return p.res, nil
	}
	return providers.Result{}, fmt.Errorf("fileProvider: %q not found", id)
}
func (p *fileProvider) Download(_ context.Context, _ string, w io.Writer) error {
	f, err := os.Open(p.path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(w, f)
	return err
}

func TestRemoteSearchUsesOnlyFirstProvider(t *testing.T) {
	store := testutil.NewStore(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	// The first provider by order is the primary; the second is never queried.
	registry := NewProviderRegistry()
	registry.Register(&fakeProvider{name: "beta", results: []providers.Result{
		{ProviderTrackID: "b1", Title: "Song B1", Artist: "B"},
		{ProviderTrackID: "dup", Title: "Dup", Artist: "B", MBID: "mbid-local"},
	}})
	registry.Register(&fakeProvider{name: "alpha", results: []providers.Result{
		{ProviderTrackID: "a1", Title: "Song A1", Artist: "A"},
	}})

	svc := NewCatalogService(CatalogServiceConfig{
		Catalog: store.Catalog, Downloads: store.Downloads, Registry: registry,
		Settings: StaticProviderSettings{}, Logger: logger,
	})

	// Seed a local track with the MBID so the "dup" remote result is filtered.
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: "ar", Name: "B"})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: "al", Name: "Al", ArtistID: artistID})
	_, _ = store.Catalog.UpsertTrack(ctx, models.Track{ID: "t", Title: "Dup", AlbumID: albumID, ArtistID: artistID, Path: "/x.mp3", MBID: "mbid-local"})

	out, err := svc.RemoteSearch(ctx, "song", 10)
	if err != nil {
		t.Fatal(err)
	}
	// Only the first provider (beta) is queried; "dup" filtered by local MBID.
	for _, tr := range out {
		if tr.Provider != "beta" {
			t.Fatalf("search must use only the first provider, got %q", tr.Provider)
		}
		if tr.Title == "Dup" {
			t.Fatal("local-duplicate remote result should be filtered by MBID")
		}
		if tr.Title == "Song A1" {
			t.Fatal("a non-primary provider must not be queried")
		}
	}
	if len(out) != 1 || out[0].Title != "Song B1" {
		t.Fatalf("expected only [Song B1], got %+v", out)
	}
}

func newOnDemand(t *testing.T) (*CatalogService, *persistence.Store, string) {
	t.Helper()
	if !testutil.FFmpegAvailable() {
		t.Skip("ffmpeg required")
	}
	store := testutil.NewStore(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Remote provider catalog (not part of the local library).
	remoteDir := t.TempDir()
	remoteFile := filepath.Join(remoteDir, "track.mp3")
	testutil.GenerateAudio(t, remoteFile, testutil.AudioTags{
		Title: "Remote Song", Artist: "Remote Artist", Album: "Remote Album", Track: 1, MBID: "mbid-remote-1",
	})

	registry := NewProviderRegistry()
	registry.Register(&fileProvider{
		name: "files",
		path: remoteFile,
		res: providers.Result{
			ProviderTrackID: "remote-1",
			Title:           "Remote Song",
			Artist:          "Remote Artist",
			Album:           "Remote Album",
			TrackNo:         1,
			MBID:            "mbid-remote-1",
			Suffix:          "mp3",
		},
	})

	downloadDir := filepath.Join(t.TempDir(), "library")
	coversDir := filepath.Join(t.TempDir(), "covers")
	scan := scanner.New(store.Catalog, store.Genres, scanner.NewExtractor("ffprobe"), coversDir, logger)

	svc := NewCatalogService(CatalogServiceConfig{
		Catalog:     store.Catalog,
		Downloads:   store.Downloads,
		Registry:    registry,
		Scanner:     scan,
		Settings:    StaticProviderSettings{AutoDownload: true},
		DownloadDir: downloadDir,
		FFmpegPath:  "ffmpeg",
		Logger:      logger,
	})
	return svc, store, downloadDir
}

func TestOnDemandSearchDownloadAndDedup(t *testing.T) {
	svc, store, _ := newOnDemand(t)
	ctx := context.Background()

	// 1. Searching for an absent track surfaces a remote result.
	remote, err := svc.RemoteSearch(ctx, "Remote", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(remote) != 1 {
		t.Fatalf("expected 1 remote result, got %d", len(remote))
	}
	if !remote[0].Remote || !IsRemoteID(remote[0].ID) {
		t.Fatalf("result is not marked remote: %+v", remote[0])
	}

	// 2. Resolving (playing) it downloads it and makes it local.
	track, local, _, err := svc.Resolve(ctx, "user-1", remote[0].ID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !local {
		t.Fatal("track should be local after resolve")
	}
	if track.Remote {
		t.Fatal("resolved track still marked remote")
	}
	_, _, tracks, _ := store.Catalog.Stats(ctx)
	if tracks != 1 {
		t.Fatalf("expected 1 local track after download, got %d", tracks)
	}

	// 3. Second access is served locally without a new download (no duplicate).
	track2, local2, _, err := svc.Resolve(ctx, "user-1", remote[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !local2 || track2.ID != track.ID {
		t.Fatalf("second resolve produced a different track: %s vs %s", track.ID, track2.ID)
	}
	_, _, tracks2, _ := store.Catalog.Stats(ctx)
	if tracks2 != 1 {
		t.Fatalf("duplicate created on second access: %d tracks", tracks2)
	}
	jobs, _ := store.Downloads.ListByUser(ctx, "user-1", 10)
	if len(jobs) != 1 {
		t.Fatalf("expected exactly 1 download job, got %d", len(jobs))
	}

	// 4. Now that the track is local, search no longer offers it remotely (dedup).
	remote2, err := svc.RemoteSearch(ctx, "Remote", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(remote2) != 0 {
		t.Fatalf("expected remote result to be deduped away, got %d", len(remote2))
	}
}
