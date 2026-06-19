package immerle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// newAdminTracksEnv builds a server with an admin + a non-admin ("bob") and a
// real covers dir, for the admin track-management endpoints. seedTrack,
// doMultipart and login are shared with the local-uploads tests.
func newAdminTracksEnv(t *testing.T) (*httptest.Server, *persistence.Store, string) {
	t.Helper()
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(ctx, "admin", "adminpw", "", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateUser(ctx, "bob", "bobpw", "", "", false); err != nil {
		t.Fatal(err)
	}
	coversDir := t.TempDir()
	h := NewHandler(Deps{
		Auth: auth, Users: store.Users, Catalog: store.Catalog,
		CoversDir: coversDir, Logger: testutil.NewLogger(),
	})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, store, coversDir
}

func TestAdminTracksRequiresAdmin(t *testing.T) {
	srv, _, _ := newAdminTracksEnv(t)
	bob := login(t, srv, "bob")
	if status := doStatus(t, srv, http.MethodGet, "/admin/tracks", bob, nil); status != http.StatusForbidden {
		t.Fatalf("non-admin should get 403, got %d", status)
	}
}

func TestAdminTracksListEditCoverDelete(t *testing.T) {
	srv, store, coversDir := newAdminTracksEnv(t)
	ctx := context.Background()
	admin := login(t, srv, "admin")

	// A scanned track not uploaded by anyone — admin manages it anyway.
	id := seedTrack(t, store, "One More Time", "")
	tr, _ := store.Catalog.GetTrack(ctx, id)
	if err := os.WriteFile(tr.Path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	// List returns the track.
	status, body := doMap(t, srv, http.MethodGet, "/admin/tracks", admin, nil)
	if status != http.StatusOK || body["total"].(float64) != 1 {
		t.Fatalf("list failed: %d %+v", status, body)
	}

	// Search miss returns nothing.
	if _, miss := doMap(t, srv, http.MethodGet, "/admin/tracks?query=zzz", admin, nil); miss["total"].(float64) != 0 {
		t.Fatalf("expected no search hits, got %v", miss["total"])
	}

	// Edit metadata.
	status, body = doMap(t, srv, http.MethodPatch, "/admin/tracks/"+id, admin, map[string]any{"title": "Aerodynamic", "year": 2001})
	if status != http.StatusOK || body["title"] != "Aerodynamic" {
		t.Fatalf("edit failed: %d %+v", status, body)
	}

	// Replace cover via upload (multipart field "file").
	png := []byte("\x89PNG\r\n\x1a\n" + "rest-of-image-bytes")
	resp := doMultipart(t, srv, http.MethodPut, "/admin/tracks/"+id+"/cover", admin, "cover.png", png)
	var view map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&view)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cover upload status %d", resp.StatusCode)
	}
	coverID, _ := view["coverArt"].(string)
	if coverID == "" || coverID == id {
		t.Fatalf("coverArt should be a fresh id, got %q", coverID)
	}
	if _, err := os.Stat(coverPath(coversDir, coverID)); err != nil {
		t.Fatalf("cover file not written: %v", err)
	}

	// Delete removes the row and the file.
	if status := doStatus(t, srv, http.MethodDelete, "/admin/tracks/"+id, admin, nil); status != http.StatusNoContent {
		t.Fatalf("delete status %d", status)
	}
	if _, err := os.Stat(tr.Path); !os.IsNotExist(err) {
		t.Fatalf("file should be deleted, stat err = %v", err)
	}
	if _, err := store.Catalog.GetTrack(ctx, id); err != persistence.ErrNotFound {
		t.Fatalf("track should be gone, got %v", err)
	}
}
