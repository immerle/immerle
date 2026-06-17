package stream

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCoverServiceServesSidecarImage(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	// Album folder with a track that has NO embedded art, plus a cover.jpg sidecar.
	libDir := t.TempDir()
	albumDir := filepath.Join(libDir, "Artist", "Album")
	if err := os.MkdirAll(albumDir, 0o755); err != nil {
		t.Fatal(err)
	}
	audioPath := filepath.Join(albumDir, "01.mp3")
	if err := os.WriteFile(audioPath, []byte("not real audio, no embedded art"), 0o644); err != nil {
		t.Fatal(err)
	}
	cover := jpegBytes(t)
	if err := os.WriteFile(filepath.Join(albumDir, "cover.jpg"), cover, 0o644); err != nil {
		t.Fatal(err)
	}

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: "ar", Name: "Artist"})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: "al", Name: "Album", ArtistID: artistID})
	_, _ = store.Catalog.UpsertTrack(ctx, models.Track{ID: "t", Title: "T", AlbumID: albumID, ArtistID: artistID, Path: audioPath})

	svc := NewCoverService(store.Catalog, filepath.Join(t.TempDir(), "covers"))

	// Original (no resize) should return the sidecar JPEG.
	data, ct, err := svc.Get(ctx, albumID, 0)
	if err != nil {
		t.Fatalf("expected sidecar cover, got error: %v", err)
	}
	if ct != "image/jpeg" || !bytes.Equal(data, cover) {
		t.Fatalf("unexpected cover: ct=%s len=%d", ct, len(data))
	}

	// Resized variant should also resolve (re-encoded JPEG).
	resized, ct, err := svc.Get(ctx, albumID, 64)
	if err != nil || ct != "image/jpeg" || len(resized) == 0 {
		t.Fatalf("resize from sidecar failed: ct=%s err=%v", ct, err)
	}

	// And via the track id too.
	if _, _, err := svc.Get(ctx, "t", 0); err != nil {
		t.Fatalf("sidecar should resolve via track id: %v", err)
	}
}

func TestCoverServiceServesRemoteImage(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	cover := jpegBytes(t)

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write(cover)
	}))
	defer srv.Close()

	svc := NewCoverService(store.Catalog, filepath.Join(t.TempDir(), "covers"))
	svc.AllowImageHosts("127.0.0.1") // httptest host

	id := models.RemoteCoverID(srv.URL + "/cover.jpg")
	data, ct, err := svc.Get(ctx, id, 0)
	if err != nil || ct != "image/jpeg" || !bytes.Equal(data, cover) {
		t.Fatalf("remote cover not served: ct=%s err=%v", ct, err)
	}
	// Cached: a second request does not hit the origin again.
	if _, _, err := svc.Get(ctx, id, 0); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("expected remote image cached (1 hit), got %d", hits)
	}
}

func TestCoverServiceRemoteHostNotAllowed(t *testing.T) {
	store := testutil.NewStore(t)
	svc := NewCoverService(store.Catalog, filepath.Join(t.TempDir(), "covers"))
	// Default allowlist is dzcdn.net only — an internal URL must be refused (SSRF guard).
	id := models.RemoteCoverID("http://169.254.169.254/latest/meta-data/")
	if _, _, err := svc.Get(context.Background(), id, 0); err == nil {
		t.Fatal("disallowed remote host must be refused")
	}
}

func TestCoverServiceNoCover(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	svc := NewCoverService(store.Catalog, filepath.Join(t.TempDir(), "covers"))
	if _, _, err := svc.Get(ctx, "missing", 0); err == nil {
		t.Fatal("expected ErrNoCover for unknown id")
	}
}
