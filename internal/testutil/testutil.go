// Package testutil provides shared fixtures and helpers for tests: an isolated
// migrated database, a store, and ffmpeg-generated audio files with tags.
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/persistence"
)

// NewLogger returns a warn-level logger for tests.
func NewLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// NewDB opens an isolated, migrated SQLite database in a temp directory. Set
// IMMERLE_TEST_DB_DSN to a Postgres DSN to run against Postgres instead, each
// test getting its own database cloned from a migrated template.
func NewDB(t *testing.T) *db.DB {
	t.Helper()
	driver, dsn := "sqlite", filepath.Join(t.TempDir(), "test.db")
	if pg := os.Getenv("IMMERLE_TEST_DB_DSN"); pg != "" {
		driver, dsn = "postgres", newPostgresDB(t, pg)
	}
	database, err := db.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if database.Dialect == "sqlite" {
		// A throwaway test DB needs no crash durability: skipping the fsync on
		// every commit is what makes the suite fast on Linux CI runners.
		if _, err := database.Exec("PRAGMA synchronous = OFF"); err != nil {
			t.Fatalf("disable sqlite sync: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

// NewStore returns a store over a fresh database.
// newPostgresDB clones a fresh database from a pre-migrated template: copying
// it is several times faster than replaying every migration per test.
func newPostgresDB(t *testing.T, dsn string) string {
	t.Helper()
	tmpl, err := postgresTemplate(dsn)
	if err != nil {
		t.Fatalf("postgres template: %v", err)
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer func() { _ = admin.Close() }()
	name := fmt.Sprintf("test_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name + " TEMPLATE " + tmpl); err != nil {
		t.Fatalf("create database: %v", err)
	}
	// Registered before NewDB's Close cleanup, so it runs after it.
	t.Cleanup(func() {
		if c, err := sql.Open("pgx", dsn); err == nil {
			_, _ = c.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
			_ = c.Close()
		}
	})
	return withDatabase(t, dsn, name)
}

var pgTemplate struct {
	once sync.Once
	name string
	err  error
}

// postgresTemplate returns a database holding the migrated schema, shared by
// every test package (they run as parallel processes) and kept across runs:
// its name carries the migrations fingerprint, so a migration change builds a
// new one. ponytail: Go migrations are not fingerprinted; drop the immerle_tmpl_*
// databases after editing one in place.
func postgresTemplate(dsn string) (string, error) {
	pgTemplate.once.Do(func() {
		pgTemplate.name, pgTemplate.err = buildPostgresTemplate(dsn)
	})
	return pgTemplate.name, pgTemplate.err
}

func buildPostgresTemplate(dsn string) (string, error) {
	name := "immerle_tmpl_" + db.MigrationsFingerprint()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		return "", err
	}
	defer func() { _ = admin.Close() }()
	conn, err := admin.Conn(ctx) // advisory locks are per session
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext('immerle_test_template'))"); err != nil {
		return "", err
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(hashtext('immerle_test_template'))")
	}()

	var exists bool
	if err := conn.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return name, nil
	}
	// Migrate under a scratch name and rename at the end, so an interrupted
	// run never leaves a half-migrated template behind.
	building := name + "_building"
	if _, err := conn.ExecContext(ctx, "DROP DATABASE IF EXISTS "+building+" WITH (FORCE)"); err != nil {
		return "", err
	}
	if _, err := conn.ExecContext(ctx, "CREATE DATABASE "+building); err != nil {
		return "", err
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + building
	tmpl, err := db.Open("postgres", u.String())
	if err != nil {
		return "", err
	}
	err = tmpl.Migrate(ctx)
	_ = tmpl.Close()
	if err != nil {
		return "", err
	}
	if _, err := conn.ExecContext(ctx, "ALTER DATABASE "+building+" RENAME TO "+name); err != nil {
		return "", err
	}
	return name, nil
}

func withDatabase(t *testing.T, dsn, name string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse postgres dsn: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

// NewStore returns a persistence store backed by NewDB.
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
	ISRC        string
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
	if tags.ISRC != "" {
		args = append(args, "-metadata", "ISRC="+tags.ISRC)
	}
	args = append(args, path)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("generate audio: %v: %s", err, out)
	}
}
