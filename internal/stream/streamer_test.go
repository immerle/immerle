package stream

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/platform/config"
)

func TestServeProgressiveTranscode(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.flac")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "sine=duration=2", src).CombinedOutput(); err != nil {
		t.Fatalf("make fixture: %v %s", err, out)
	}
	s := NewStreamer(config.TranscodeConfig{FFmpegPath: ffmpeg, CacheDir: filepath.Join(dir, "cache")}, slog.New(slog.DiscardHandler))
	track := models.Track{ID: "t1", Path: src, Suffix: "flac"}
	opts := Options{MaxBitRate: 128, Format: "mp3"}

	// First listen from byte 0: streamed while ffmpeg runs, not range-seekable.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Range", "bytes=0-")
	if err := s.Serve(rec, req, track, opts); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || rec.Header().Get("Accept-Ranges") != "none" || rec.Body.Len() == 0 {
		t.Fatalf("progressive: code=%d ranges=%q len=%d", rec.Code, rec.Header().Get("Accept-Ranges"), rec.Body.Len())
	}
	cached, err := os.ReadFile(filepath.Join(dir, "cache", "t1_mp3_128.mp3"))
	if err != nil || len(cached) != rec.Body.Len() {
		t.Fatalf("cache: err=%v len=%d want %d", err, len(cached), rec.Body.Len())
	}

	// Later requests hit the cache with full range support.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Range", "bytes=10-")
	if err := s.Serve(rec, req, track, opts); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusPartialContent || rec.Body.Len() != len(cached)-10 {
		t.Fatalf("cached range: code=%d len=%d", rec.Code, rec.Body.Len())
	}
}
