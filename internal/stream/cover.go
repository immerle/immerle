// Package stream serves audio (with range/transcode support) and cover art.
package stream

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"github.com/dhowden/tag"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// CoverService resolves, resizes and caches cover art.
type CoverService struct {
	catalog   *persistence.CatalogRepo
	coversDir string
	cacheDir  string
	http      *http.Client
	// allowHosts restricts which hosts a remote cover URL may point at (SSRF
	// guard). Matched by exact host or as a dotted suffix.
	allowHosts []string
}

// NewCoverService builds a CoverService. coversDir holds embedded art extracted
// at scan time; resized variants are cached under coversDir/cache.
func NewCoverService(catalog *persistence.CatalogRepo, coversDir string) *CoverService {
	return &CoverService{
		catalog:    catalog,
		coversDir:  coversDir,
		cacheDir:   filepath.Join(coversDir, "cache"),
		http:       &http.Client{Timeout: 20 * time.Second},
		allowHosts: []string{"dzcdn.net"},
	}
}

// AllowImageHosts adds host suffixes that remote cover URLs may point at.
func (c *CoverService) AllowImageHosts(hosts ...string) {
	for _, h := range hosts {
		if h = strings.TrimSpace(strings.ToLower(h)); h != "" {
			c.allowHosts = append(c.allowHosts, h)
		}
	}
}

// ErrNoCover indicates no cover art could be resolved for an id.
var ErrNoCover = errors.New("no cover art")

// Get returns cover art bytes (optionally resized to a square of `size` px) and a
// content type. size <= 0 returns the original.
func (c *CoverService) Get(ctx context.Context, id string, size int) ([]byte, string, error) {
	if id == "" {
		return nil, "", ErrNoCover
	}

	if size > 0 {
		if cached, err := os.ReadFile(c.cachePath(id, size)); err == nil {
			return cached, "image/jpeg", nil
		}
	}

	raw, err := c.resolveOriginal(ctx, id)
	if err != nil {
		return nil, "", err
	}

	if size <= 0 {
		return raw, detectContentType(raw), nil
	}

	resized, err := resizeSquareJPEG(raw, size)
	if err != nil {
		// Fall back to the original if decoding/resizing fails.
		return raw, detectContentType(raw), nil
	}
	_ = os.MkdirAll(c.cacheDir, 0o755)
	_ = os.WriteFile(c.cachePath(id, size), resized, 0o644)
	return resized, "image/jpeg", nil
}

// resolveOriginal finds the source image bytes for an id, which may reference an
// album/track cover file, embedded/sidecar art, or a remote provider image URL.
func (c *CoverService) resolveOriginal(ctx context.Context, id string) ([]byte, error) {
	// Remote provider image (e.g. a Deezer CDN cover/avatar).
	if models.IsRemoteCoverID(id) {
		return c.fetchRemoteCover(ctx, id)
	}

	// Direct cover file written at scan time (keyed by album id).
	if data, err := os.ReadFile(filepath.Join(c.coversDir, id)); err == nil {
		return data, nil
	}

	// Try as a track id: embedded art, then a sidecar image in its directory.
	if t, err := c.catalog.GetTrack(ctx, id); err == nil {
		if data, err := embeddedPicture(t.Path); err == nil {
			return data, nil
		}
		if data, err := sidecarCover(t.Path); err == nil {
			return data, nil
		}
		// Track may point at an album cover.
		if t.CoverArt != "" && t.CoverArt != id {
			if data, err := os.ReadFile(filepath.Join(c.coversDir, t.CoverArt)); err == nil {
				return data, nil
			}
		}
	}

	// Try as an album id: a sidecar image in the album folder, else embedded art
	// from one of its tracks.
	if tracks, err := c.catalog.ListTracksByAlbum(ctx, id); err == nil && len(tracks) > 0 {
		if data, err := sidecarCover(tracks[0].Path); err == nil {
			return data, nil
		}
		for _, t := range tracks {
			if data, err := embeddedPicture(t.Path); err == nil {
				return data, nil
			}
		}
	}

	return nil, ErrNoCover
}

// sidecarCoverNames are the conventional cover-image filenames (without
// extension) looked up next to audio files, in priority order.
var sidecarCoverNames = []string{"cover", "folder", "front", "albumart", "album", "thumb"}

var sidecarCoverExts = []string{".jpg", ".jpeg", ".png", ".webp", ".gif"}

// sidecarCover finds a cover image stored as a separate file in the directory of
// the given audio file (and its parent, for disc-subfolder layouts).
func sidecarCover(audioPath string) ([]byte, error) {
	if audioPath == "" {
		return nil, ErrNoCover
	}
	dir := filepath.Dir(audioPath)
	dirs := []string{dir}
	if parent := filepath.Dir(dir); parent != dir {
		dirs = append(dirs, parent)
	}
	for _, d := range dirs {
		if data, err := findSidecar(d); err == nil {
			return data, nil
		}
	}
	return nil, ErrNoCover
}

func findSidecar(dir string) ([]byte, error) {
	// Exact conventional names first.
	for _, name := range sidecarCoverNames {
		for _, ext := range sidecarCoverExts {
			p := filepath.Join(dir, name+ext)
			if data, err := os.ReadFile(p); err == nil && looksLikeImageBytes(data) {
				return data, nil
			}
		}
	}
	// Fallback: any image file in the directory (case-insensitive scan).
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, ErrNoCover
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		for _, want := range sidecarCoverExts {
			if ext == want {
				if data, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil && looksLikeImageBytes(data) {
					return data, nil
				}
			}
		}
	}
	return nil, ErrNoCover
}

func looksLikeImageBytes(data []byte) bool {
	return detectContentType(data) != "application/octet-stream"
}

func (c *CoverService) cachePath(id string, size int) string {
	return filepath.Join(c.cacheDir, sanitizeID(id)+"_"+strconv.Itoa(size)+".jpg")
}

// sanitizeID makes an id safe for use as a cache filename.
func sanitizeID(id string) string {
	return strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(id)
}

// fetchRemoteCover downloads a provider image URL (host-allowlisted to prevent
// SSRF) and caches the original under the cover cache.
func (c *CoverService) fetchRemoteCover(ctx context.Context, id string) ([]byte, error) {
	imageURL, ok := models.DecodeRemoteCoverID(id)
	if !ok {
		return nil, ErrNoCover
	}
	u, err := url.Parse(imageURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !c.hostAllowed(u.Hostname()) {
		return nil, ErrNoCover
	}

	cache := filepath.Join(c.cacheDir, "remote_"+sanitizeID(strings.TrimPrefix(id, models.RemoteCoverPrefix)))
	if data, err := os.ReadFile(cache); err == nil {
		return data, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, ErrNoCover
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
	if err != nil || !looksLikeImageBytes(data) {
		return nil, ErrNoCover
	}
	_ = os.MkdirAll(c.cacheDir, 0o755)
	_ = os.WriteFile(cache, data, 0o644)
	return data, nil
}

func (c *CoverService) hostAllowed(host string) bool {
	host = strings.ToLower(host)
	for _, allowed := range c.allowHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// embeddedPicture extracts embedded cover art from an audio file.
func embeddedPicture(path string) ([]byte, error) {
	if path == "" {
		return nil, ErrNoCover
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, err
	}
	if pic := m.Picture(); pic != nil && len(pic.Data) > 0 {
		return pic.Data, nil
	}
	return nil, ErrNoCover
}

// resizeSquareJPEG decodes raw, scales it to fit within size×size preserving
// aspect ratio, and re-encodes as JPEG.
func resizeSquareJPEG(raw []byte, size int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, errors.New("empty image")
	}
	// Scale so the larger dimension becomes `size`.
	nw, nh := size, size
	if w > h {
		nh = h * size / w
	} else {
		nw = w * size / h
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func detectContentType(data []byte) string {
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	if len(data) >= 8 && string(data[1:4]) == "PNG" {
		return "image/png"
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return "image/gif"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	return "application/octet-stream"
}
