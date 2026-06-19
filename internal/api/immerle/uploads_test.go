package immerle

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/scanner"
	"github.com/immerle/immerle/internal/testutil"
)

// seedTrack inserts a track owned by ownerID (empty = not uploaded).
func seedTrack(t *testing.T, store *persistence.Store, title, ownerID string) string {
	t.Helper()
	ctx := context.Background()
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: time.Now()})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: time.Now()})
	id, err := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: title, AlbumID: albumID, ArtistID: artistID,
		Path: "/" + uuid.NewString() + ".mp3", Duration: 100, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ownerID != "" {
		if err := store.Catalog.SetTrackOwner(ctx, id, ownerID); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func newLibraryServer(t *testing.T) (*httptest.Server, *persistence.Store, *core.AuthService, string) {
	t.Helper()
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	for _, u := range []string{"alice", "bob"} {
		if _, err := auth.CreateUser(ctx, u, u+"pw", "", "", false); err != nil {
			t.Fatal(err)
		}
	}
	coversDir := t.TempDir()
	scan := scanner.New(store.Catalog, store.Genres, scanner.NewExtractor("ffprobe"), coversDir, testutil.NewLogger())
	h := NewHandler(Deps{
		Auth: auth, Users: store.Users, Catalog: store.Catalog,
		Scanner: scan, UploadsDir: t.TempDir(), CoversDir: coversDir, Logger: testutil.NewLogger(),
	})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, store, auth, coversDir
}

// doMultipart posts a single multipart file field "file".
func doMultipart(t *testing.T, srv *httptest.Server, method, path, token, filename string, content []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(content)
	_ = mw.Close()
	req, err := http.NewRequest(method, srv.URL+apiBase+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestLocalSongsIsolatedPerUser(t *testing.T) {
	srv, store, _, _ := newLibraryServer(t)
	alice := login(t, srv, "alice")
	bob := login(t, srv, "bob")
	aliceID, _ := store.Users.GetByUsername(context.Background(), "alice")
	bobID, _ := store.Users.GetByUsername(context.Background(), "bob")

	seedTrack(t, store, "alice song", aliceID.ID)
	seedTrack(t, store, "bob song", bobID.ID)
	seedTrack(t, store, "scanned", "") // not uploaded by anyone

	status, body := doMap(t, srv, http.MethodGet, "/library/local", alice, nil)
	if status != http.StatusOK {
		t.Fatalf("local list status %d: %+v", status, body)
	}
	songs, _ := body["songs"].([]any)
	if len(songs) != 1 {
		t.Fatalf("alice should see 1 local song, got %d: %+v", len(songs), body)
	}
	if s := songs[0].(map[string]any); s["title"] != "alice song" {
		t.Fatalf("unexpected song: %+v", s)
	}

	// bob sees only his.
	_, bbody := doMap(t, srv, http.MethodGet, "/library/local", bob, nil)
	bsongs, _ := bbody["songs"].([]any)
	if len(bsongs) != 1 || bsongs[0].(map[string]any)["title"] != "bob song" {
		t.Fatalf("bob local list wrong: %+v", bbody)
	}
}

func TestRenameTrackOwnerOnly(t *testing.T) {
	srv, store, _, _ := newLibraryServer(t)
	alice := login(t, srv, "alice")
	bob := login(t, srv, "bob")
	aliceID, _ := store.Users.GetByUsername(context.Background(), "alice")
	id := seedTrack(t, store, "old", aliceID.ID)

	// Owner renames.
	status, body := doMap(t, srv, http.MethodPatch, "/library/tracks/"+id, alice, map[string]any{"title": "new name"})
	if status != http.StatusOK || body["title"] != "new name" {
		t.Fatalf("rename failed: status %d %+v", status, body)
	}

	// Non-owner is forbidden.
	if status, _ := doMap(t, srv, http.MethodPatch, "/library/tracks/"+id, bob, map[string]any{"title": "hijack"}); status != http.StatusForbidden {
		t.Fatalf("expected 403 for non-owner, got %d", status)
	}
	// Empty title rejected.
	if status, _ := doMap(t, srv, http.MethodPatch, "/library/tracks/"+id, alice, map[string]any{"title": "  "}); status != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty title, got %d", status)
	}
	// Missing track.
	if status, _ := doMap(t, srv, http.MethodPatch, "/library/tracks/nope", alice, map[string]any{"title": "x"}); status != http.StatusNotFound {
		t.Fatalf("expected 404 for missing track, got %d", status)
	}
}

func TestSetTrackCover(t *testing.T) {
	srv, store, _, coversDir := newLibraryServer(t)
	alice := login(t, srv, "alice")
	aliceID, _ := store.Users.GetByUsername(context.Background(), "alice")
	id := seedTrack(t, store, "song", aliceID.ID)

	png := []byte("\x89PNG\r\n\x1a\n" + "rest-of-image-bytes")
	resp := doMultipart(t, srv, http.MethodPut, "/library/tracks/"+id+"/cover", alice, "cover.png", png)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cover upload status %d", resp.StatusCode)
	}
	var view map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&view)
	coverID, _ := view["coverArt"].(string)
	if coverID == "" || coverID == id {
		t.Fatalf("coverArt should be a fresh id, got %q", coverID)
	}
	// The cover file was written under coversDir keyed by the new id.
	if _, err := os.Stat(filepath.Join(coversDir, coverID)); err != nil {
		t.Fatalf("cover file not written: %v", err)
	}
	// DB reflects the new cover.
	tr, _ := store.Catalog.GetTrack(context.Background(), id)
	if tr.CoverArt != coverID {
		t.Fatalf("track cover_art not updated: %q", tr.CoverArt)
	}

	// Non-image rejected.
	bad := doMultipart(t, srv, http.MethodPut, "/library/tracks/"+id+"/cover", alice, "x.png", []byte("not an image"))
	bad.Body.Close()
	if bad.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for non-image, got %d", bad.StatusCode)
	}
}

func TestUploadValidation(t *testing.T) {
	srv, _, _, _ := newLibraryServer(t)
	alice := login(t, srv, "alice")

	// Non-audio extension is rejected before any ingest.
	resp := doMultipart(t, srv, http.MethodPost, "/library/uploads", alice, "notes.txt", []byte("hello"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for non-audio, got %d", resp.StatusCode)
	}
}
