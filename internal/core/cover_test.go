package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gossignol/gossignol/internal/providers"
	"github.com/gossignol/gossignol/internal/testutil"
)

func TestSaveSidecarCover(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0} // sniffs as image/jpeg
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jpeg)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	dest := filepath.Join(dir, "Artist", "Album", "01 - Song.mp3")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}

	svc := &CatalogService{state: &catalogServiceState{logger: testutil.NewLogger()}}
	svc.saveSidecarCover(context.Background(), providers.Result{CoverImageURL: srv.URL}, dest)

	cover := filepath.Join(filepath.Dir(dest), "cover.jpg")
	got, err := os.ReadFile(cover)
	if err != nil {
		t.Fatalf("cover.jpg not written: %v", err)
	}
	if len(got) != len(jpeg) {
		t.Fatalf("cover size mismatch: got %d want %d", len(got), len(jpeg))
	}
}

func TestSaveSidecarCoverSkipsNonImageAndExisting(t *testing.T) {
	// Non-image response must not be saved.
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>nope</html>"))
	}))
	t.Cleanup(html.Close)

	dir := t.TempDir()
	dest := filepath.Join(dir, "01 - Song.mp3")
	svc := &CatalogService{state: &catalogServiceState{logger: testutil.NewLogger()}}
	svc.saveSidecarCover(context.Background(), providers.Result{CoverImageURL: html.URL}, dest)
	if _, err := os.Stat(filepath.Join(dir, "cover.jpg")); !os.IsNotExist(err) {
		t.Fatal("non-image must not be saved as cover")
	}

	// An existing cover is left untouched (no overwrite).
	cover := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(cover, []byte("KEEP"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.saveSidecarCover(context.Background(), providers.Result{CoverImageURL: html.URL}, dest)
	if b, _ := os.ReadFile(cover); string(b) != "KEEP" {
		t.Fatal("existing cover should not be overwritten")
	}
}
