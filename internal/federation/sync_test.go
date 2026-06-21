package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	fed := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, nil, testLogger())
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

	// Make it private → delete on the hub.
	p, _ := store.Playlists.Get(ctx, plID)
	p.Public = false
	_ = store.Playlists.UpdateMeta(ctx, p)
	if err := s.handle(ctx, job); err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	if !strings.HasSuffix(st.deleted, "/"+plID) {
		t.Fatalf("expected DELETE for /%s, got %q", plID, st.deleted)
	}
	st.mu.Unlock()
}
