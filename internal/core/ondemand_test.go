package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/services/ondemand"
	"github.com/immerle/immerle/internal/services/ondemand/providers"
	"github.com/immerle/immerle/internal/services/scanner"
	"github.com/immerle/immerle/internal/testutil"
)

// fileProvider serves a single generated audio file as a remote track.
type fileProvider struct {
	path string
	res  providers.Result
}

func (p *fileProvider) Name() string       { return "files" }
func (p *fileProvider) MaxQuality() string { return "test" }
func (p *fileProvider) Search(context.Context, string, int) ([]providers.Result, error) {
	return []providers.Result{p.res}, nil
}
func (p *fileProvider) Resolve(context.Context, string) (providers.Result, error) {
	return p.res, nil
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

// newOnDemand builds an on-demand catalog over one downloadable remote track.
func newOnDemand(t *testing.T) (*ondemand.CatalogService, *persistence.Store) {
	t.Helper()
	if !testutil.FFmpegAvailable() {
		t.Skip("ffmpeg required")
	}
	store := testutil.NewStore(t)
	remoteFile := filepath.Join(t.TempDir(), "track.mp3")
	testutil.GenerateAudio(t, remoteFile, testutil.AudioTags{Title: "Remote Song", Artist: "Remote Artist", Album: "Remote Album", Track: 1, MBID: "mbid-remote-1"})

	registry := ondemand.NewProviderRegistry()
	registry.Register(&fileProvider{path: remoteFile, res: providers.Result{
		ProviderTrackID: "remote-1", Title: "Remote Song", Artist: "Remote Artist", Album: "Remote Album",
		TrackNo: 1, MBID: "mbid-remote-1", Suffix: "mp3",
	}})
	coversDir := filepath.Join(t.TempDir(), "covers")
	logger := testutil.NewLogger()
	return ondemand.NewCatalogService(ondemand.CatalogServiceConfig{
		Catalog:     store.Catalog,
		Downloads:   store.Downloads,
		Registry:    registry,
		Scanner:     scanner.New(store.Catalog, store.Genres, scanner.NewExtractor("ffprobe"), coversDir, logger),
		Settings:    ondemand.StaticProviderSettings{AutoDownload: true},
		DownloadDir: filepath.Join(t.TempDir(), "library"),
		FFmpegPath:  "ffmpeg",
		Logger:      logger,
	}), store
}

// TestHallOfFameAddResolvesRemoteTrack: a "remote:" track id (no row in
// `tracks` yet) used to fail with an opaque FK constraint error, since
// hall_of_fame_entries has a foreign key on track_id. Add/SetOrder must
// resolve such ids first, like PlaylistService already does.
