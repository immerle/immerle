package scanner

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

// buildLibrary writes a small fixture library: 2 artists, 3 albums, 5 tracks.
func buildLibrary(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mk := func(rel string, tags testutil.AudioTags) {
		p := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		testutil.GenerateAudio(t, p, tags)
	}
	mk("Artist A/Album 1/01.mp3", testutil.AudioTags{Title: "A1T1", Artist: "Artist A", Album: "Album 1", Track: 1, Genre: "Rock", Year: 2001, MBID: "mbid-a1t1"})
	mk("Artist A/Album 1/02.mp3", testutil.AudioTags{Title: "A1T2", Artist: "Artist A", Album: "Album 1", Track: 2, Genre: "Rock", Year: 2001})
	mk("Artist A/Album 2/01.mp3", testutil.AudioTags{Title: "A2T1", Artist: "Artist A", Album: "Album 2", Track: 1, Genre: "Pop", Year: 2005})
	mk("Artist B/Album 3/01.mp3", testutil.AudioTags{Title: "B3T1", Artist: "Artist B", Album: "Album 3", Track: 1, Genre: "Jazz", Year: 2010})
	mk("Artist B/Album 3/02.mp3", testutil.AudioTags{Title: "B3T2", Artist: "Artist B", Album: "Album 3", Track: 2, Genre: "Jazz", Year: 2010})
	return root
}

func newScanner(t *testing.T) *Scanner {
	store := testutil.NewStore(t)
	coversDir := filepath.Join(t.TempDir(), "covers")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return New(store.Catalog, store.Genres, NewExtractor("ffprobe"), coversDir, logger)
}

func TestFullScanCounts(t *testing.T) {
	if !testutil.FFmpegAvailable() {
		t.Skip("ffmpeg not available")
	}
	root := buildLibrary(t)
	s := newScanner(t)
	store := s.catalog

	ctx := context.Background()
	res, err := s.ScanPaths(ctx, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 5 || res.Added != 5 {
		t.Fatalf("expected 5 scanned/added, got scanned=%d added=%d", res.Scanned, res.Added)
	}
	artists, albums, tracks, err := store.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if artists != 2 || albums != 3 || tracks != 5 {
		t.Fatalf("expected 2 artists / 3 albums / 5 tracks, got %d / %d / %d", artists, albums, tracks)
	}
}

func TestRescanNoDuplicates(t *testing.T) {
	if !testutil.FFmpegAvailable() {
		t.Skip("ffmpeg not available")
	}
	root := buildLibrary(t)
	s := newScanner(t)
	ctx := context.Background()

	if _, err := s.ScanPaths(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}
	res2, err := s.ScanPaths(ctx, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Added != 0 {
		t.Fatalf("rescan added %d tracks, expected 0", res2.Added)
	}
	artists, albums, tracks, _ := s.catalog.Stats(ctx)
	if artists != 2 || albums != 3 || tracks != 5 {
		t.Fatalf("rescan changed counts: %d / %d / %d", artists, albums, tracks)
	}
}

func TestRenamePreservesAnnotations(t *testing.T) {
	if !testutil.FFmpegAvailable() {
		t.Skip("ffmpeg not available")
	}
	root := buildLibrary(t)
	store := testutil.NewStore(t)
	coversDir := filepath.Join(t.TempDir(), "covers")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s := New(store.Catalog, store.Genres, NewExtractor("ffprobe"), coversDir, logger)
	ctx := context.Background()

	if _, err := s.ScanPaths(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}

	// Find the track with an MBID and star it for a user.
	_, _, tracks, _ := store.Catalog.Search(ctx, "A1T1", 10, 10, 10)
	if len(tracks) == 0 {
		t.Fatal("seed track not found")
	}
	track := tracks[0]

	user := models.User{ID: uuid.NewString(), Username: "u", PasswordHash: "x", CreatedAt: time.Now()}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := store.Annotations.SetStarred(ctx, user.ID, models.ItemTrack, track.ID, true); err != nil {
		t.Fatal(err)
	}

	// Rename the file on disk (path changes, content/MBID stable).
	oldPath := track.Path
	newPath := filepath.Join(filepath.Dir(oldPath), "renamed.mp3")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ScanPaths(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}

	// The track id must be unchanged (matched by MBID/hash), and the star kept.
	_, _, tracks2, _ := store.Catalog.Search(ctx, "A1T1", 10, 10, 10)
	if len(tracks2) != 1 {
		t.Fatalf("expected exactly 1 matching track after rename, got %d", len(tracks2))
	}
	if tracks2[0].ID != track.ID {
		t.Fatalf("track identity changed on rename: %s -> %s", track.ID, tracks2[0].ID)
	}
	ann, err := store.Annotations.Get(ctx, user.ID, models.ItemTrack, track.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ann.Starred == nil {
		t.Fatal("annotation lost after rename")
	}
}
