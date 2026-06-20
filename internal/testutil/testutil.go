// Package testutil provides shared fixtures and helpers for tests: an isolated
// migrated database, a store, and ffmpeg-generated audio files with tags.
package testutil

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/persistence"
)

// NewLogger returns a warn-level logger for tests.
func NewLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// NewDB opens an isolated, migrated SQLite database in a temp directory.
func NewDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "test.db")
	database, err := db.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

// NewStore returns a store over a fresh database.
func NewStore(t *testing.T) *persistence.Store {
	t.Helper()
	return persistence.New(NewDB(t))
}

// FFmpegAvailable reports whether ffmpeg is on PATH.
func FFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// AudioTags describe metadata to embed in a generated audio fixture.
type AudioTags struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	Composer    string
	Genre       string
	Year        int
	Track       int
	Disc        int
	BPM         int
	ReplayGain  string // track gain, e.g. "-6.50 dB"
	Work        string
	Movement    int
	Performer   string
	Producer    string
	Lyrics      string
	MBID        string
}

// GenerateAudio writes a short MP3 with the given tags to path using ffmpeg.
// It skips the test if ffmpeg is unavailable.
func GenerateAudio(t *testing.T, path string, tags AudioTags) {
	t.Helper()
	if !FFmpegAvailable() {
		t.Skip("ffmpeg not available")
	}
	args := []string{
		"-v", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-metadata", "title=" + tags.Title,
		"-metadata", "artist=" + tags.Artist,
		"-metadata", "album=" + tags.Album,
	}
	if tags.AlbumArtist != "" {
		args = append(args, "-metadata", "album_artist="+tags.AlbumArtist)
	}
	if tags.Genre != "" {
		args = append(args, "-metadata", "genre="+tags.Genre)
	}
	if tags.Year != 0 {
		args = append(args, "-metadata", fmt.Sprintf("date=%d", tags.Year))
	}
	if tags.Track != 0 {
		args = append(args, "-metadata", fmt.Sprintf("track=%d", tags.Track))
	}
	if tags.Disc != 0 {
		args = append(args, "-metadata", fmt.Sprintf("disc=%d", tags.Disc))
	}
	if tags.Composer != "" {
		args = append(args, "-metadata", "composer="+tags.Composer)
	}
	if tags.BPM != 0 {
		args = append(args, "-metadata", fmt.Sprintf("TBPM=%d", tags.BPM))
	}
	if tags.ReplayGain != "" {
		args = append(args, "-metadata", "replaygain_track_gain="+tags.ReplayGain)
	}
	if tags.Work != "" {
		args = append(args, "-metadata", "work="+tags.Work)
	}
	if tags.Movement != 0 {
		args = append(args, "-metadata", fmt.Sprintf("movement=%d", tags.Movement))
	}
	if tags.Performer != "" {
		args = append(args, "-metadata", "performer="+tags.Performer)
	}
	if tags.Producer != "" {
		args = append(args, "-metadata", "producer="+tags.Producer)
	}
	if tags.Lyrics != "" {
		args = append(args, "-metadata", "lyrics="+tags.Lyrics)
	}
	if tags.MBID != "" {
		args = append(args, "-metadata", "MUSICBRAINZ_TRACKID="+tags.MBID)
	}
	args = append(args, path)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("generate audio: %v: %s", err, out)
	}
}
