package federation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/federation/hub"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/outbox"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// fakeCovers serves the same bytes for any cover id.
type fakeCovers struct{ data []byte }

func (f fakeCovers) Get(_ context.Context, _ string, _ int) ([]byte, string, error) {
	return f.data, "image/jpeg", nil
}

// perIDCovers serves distinct bytes per cover id, for tests that need each
// tile to be individually addressable/distinguishable.
type perIDCovers struct{ byID map[string][]byte }

func (f perIDCovers) Get(_ context.Context, id string, _ int) ([]byte, string, error) {
	data, ok := f.byID[id]
	if !ok {
		return nil, "", errNoCovers
	}
	return data, "image/jpeg", nil
}

// solidJPEG encodes a tiny single-color JPEG, decodable by image.Decode (a
// real image, unlike the "JPEGDATA" placeholder used elsewhere in this file).
func solidJPEG(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type syncStub struct {
	mu       sync.Mutex
	missing  [][]string
	uploaded []string
	putBody  string
	putPath  string
	deleted  string
}

func newSyncStub(t *testing.T) (*httptest.Server, *syncStub) {
	st := &syncStub{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/covers/missing", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Hashes []string `json:"hashes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		st.mu.Lock()
		st.missing = append(st.missing, req.Hashes)
		st.mu.Unlock()
		// The hub has none of them yet → all missing.
		_ = json.NewEncoder(w).Encode(hub.PublicMissingCoversResponse{Missing: &req.Hashes})
	})
	mux.HandleFunc("/api/v1/covers/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			st.mu.Lock()
			st.uploaded = append(st.uploaded, strings.TrimPrefix(r.URL.Path, "/api/v1/covers/"))
			st.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}
	})
	mux.HandleFunc("/api/v1/instances/me/playlists/", func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			st.putBody = string(b)
			st.putPath = r.URL.Path
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case http.MethodDelete:
			st.deleted = r.URL.Path
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "deleted": true})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

func TestOutboxWorkerSyncsAndDeletesPublicPlaylist(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store := testutil.NewStore(t)

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Discovery", ArtistID: artistID, CreatedAt: now})
	trackID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "One More Time", AlbumID: albumID, ArtistID: artistID,
		Genre: "House", Year: 2001, Duration: 320, Path: "/p.mp3", CreatedAt: now, UpdatedAt: now,
	})

	plID := uuid.NewString()
	_ = store.Playlists.Create(ctx, models.Playlist{ID: plID, Name: "Summer Hits", OwnerID: owner.ID, Public: true, CreatedAt: now, UpdatedAt: now})
	_ = store.Playlists.ReplaceTracks(ctx, plID, []string{trackID}, owner.ID)

	srv, st := newSyncStub(t)
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "iml_key", SyncPlaylists: true}
	fed := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	worker := outbox.NewWorker(store.Outbox, testLogger())
	s := NewPlaylistSyncer(fed, worker, store.PlaylistSync, store.CoverUploads, store.Playlists, fakeCovers{data: []byte("JPEGDATA")}, testLogger())
	job := persistence.OutboxJob{Kind: PlaylistSyncKind, DedupeKey: plID}

	// Upsert flow.
	if err := s.handle(ctx, job); err != nil {
		t.Fatal(err)
	}

	wantHash := func() string { s := sha256.Sum256([]byte("JPEGDATA")); return hex.EncodeToString(s[:]) }()
	st.mu.Lock()
	if !strings.HasSuffix(st.putPath, "/"+plID) {
		t.Fatalf("PUT path = %q, want suffix /%s", st.putPath, plID)
	}
	if len(st.missing) != 1 || len(st.missing[0]) != 1 || st.missing[0][0] != wantHash {
		t.Fatalf("covers/missing not called with the cover hash: %+v", st.missing)
	}
	if len(st.uploaded) != 1 || st.uploaded[0] != wantHash {
		t.Fatalf("cover not uploaded: %+v", st.uploaded)
	}
	if !strings.Contains(st.putBody, `"name":"Summer Hits"`) ||
		!strings.Contains(st.putBody, `"title":"One More Time"`) ||
		!strings.Contains(st.putBody, `"cover":"/api/v1/covers/`+wantHash+`"`) ||
		!strings.Contains(st.putBody, `"seconds":320`) {
		t.Fatalf("unexpected PUT body: %s", st.putBody)
	}
	st.mu.Unlock()

	// Sync state recorded.
	if h, _ := store.PlaylistSync.Hash(ctx, plID); h == "" {
		t.Fatal("playlist_sync hash not recorded")
	}

	// Unchanged → handle again → no new PUT.
	prevPut := st.putBody
	if err := s.handle(ctx, job); err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	if st.putBody != prevPut {
		t.Fatal("unchanged playlist should not re-PUT")
	}
	st.mu.Unlock()

	// Disabling sync purges: it enqueues a delete job for every synced playlist.
	if err := s.PurgePlaylists(ctx); err != nil {
		t.Fatal(err)
	}
	pj, err := store.Outbox.ClaimNext(ctx, time.Now())
	if err != nil || pj.DedupeKey != plID {
		t.Fatalf("purge should enqueue a job for %s, got %+v err=%v", plID, pj, err)
	}

	// With sync turned off, the handler deletes the (still public) playlist.
	cfg.SyncPlaylists = false
	if err := s.handle(ctx, job); err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	if !strings.HasSuffix(st.deleted, "/"+plID) {
		t.Fatalf("expected DELETE for /%s, got %q", plID, st.deleted)
	}
	st.mu.Unlock()
}

func TestPlaylistSyncComposesMosaicWhenNoCustomCover(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store := testutil.NewStore(t)

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	trackA, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "A", AlbumID: albumID, ArtistID: artistID, CoverArt: "cover-a",
		Path: "/a.mp3", CreatedAt: now, UpdatedAt: now,
	})
	trackB, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "B", AlbumID: albumID, ArtistID: artistID, CoverArt: "cover-b",
		Path: "/b.mp3", CreatedAt: now, UpdatedAt: now,
	})

	plID := uuid.NewString()
	_ = store.Playlists.Create(ctx, models.Playlist{ID: plID, Name: "Mosaic Test", OwnerID: owner.ID, Public: true, CreatedAt: now, UpdatedAt: now})
	_ = store.Playlists.ReplaceTracks(ctx, plID, []string{trackA, trackB}, owner.ID)

	srv, st := newSyncStub(t)
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "iml_key", SyncPlaylists: true}
	fed := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	worker := outbox.NewWorker(store.Outbox, testLogger())
	covers := perIDCovers{byID: map[string][]byte{
		"cover-a": solidJPEG(t, color.RGBA{R: 255, A: 255}),
		"cover-b": solidJPEG(t, color.RGBA{B: 255, A: 255}),
	}}
	s := NewPlaylistSyncer(fed, worker, store.PlaylistSync, store.CoverUploads, store.Playlists, covers, testLogger())

	if err := s.handle(ctx, persistence.OutboxJob{Kind: PlaylistSyncKind, DedupeKey: plID}); err != nil {
		t.Fatal(err)
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	var body struct {
		Image string `json:"image"`
	}
	if err := json.Unmarshal([]byte(st.putBody), &body); err != nil {
		t.Fatalf("decode PUT body: %v (body=%s)", err, st.putBody)
	}
	if !strings.HasPrefix(body.Image, "/api/v1/covers/") {
		t.Fatalf("expected a composed mosaic cover url, got %q", body.Image)
	}
	hash := strings.TrimPrefix(body.Image, "/api/v1/covers/")
	found := false
	for _, h := range st.uploaded {
		if h == hash {
			found = true
		}
	}
	if !found {
		t.Fatalf("mosaic %s was not uploaded, uploaded=%v", hash, st.uploaded)
	}
}

func TestPlaylistSyncKeepsCustomCoverOverMosaic(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store := testutil.NewStore(t)

	owner := models.User{ID: uuid.NewString(), Username: "admin2", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	trackID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "A", AlbumID: albumID, ArtistID: artistID, CoverArt: "cover-a",
		Path: "/a.mp3", CreatedAt: now, UpdatedAt: now,
	})

	plID := uuid.NewString()
	_ = store.Playlists.Create(ctx, models.Playlist{ID: plID, Name: "Custom Cover Test", OwnerID: owner.ID, Public: true, CreatedAt: now, UpdatedAt: now})
	_ = store.Playlists.SetCover(ctx, plID, "custom-cover")
	_ = store.Playlists.ReplaceTracks(ctx, plID, []string{trackID}, owner.ID)

	srv, st := newSyncStub(t)
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "iml_key", SyncPlaylists: true}
	fed := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	worker := outbox.NewWorker(store.Outbox, testLogger())
	covers := perIDCovers{byID: map[string][]byte{
		"custom-cover": solidJPEG(t, color.RGBA{G: 255, A: 255}),
		"cover-a":      solidJPEG(t, color.RGBA{R: 255, A: 255}),
	}}
	s := NewPlaylistSyncer(fed, worker, store.PlaylistSync, store.CoverUploads, store.Playlists, covers, testLogger())

	if err := s.handle(ctx, persistence.OutboxJob{Kind: PlaylistSyncKind, DedupeKey: plID}); err != nil {
		t.Fatal(err)
	}

	wantHash := func() string {
		sum := sha256.Sum256(covers.byID["custom-cover"])
		return hex.EncodeToString(sum[:])
	}()
	st.mu.Lock()
	defer st.mu.Unlock()
	if !strings.Contains(st.putBody, `"image":"/api/v1/covers/`+wantHash+`"`) {
		t.Fatalf("expected the custom cover, not a mosaic, got body=%s", st.putBody)
	}
}
