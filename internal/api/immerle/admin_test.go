package immerle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/testutil"
)

// fakeCleanup is a stand-in CleanupController (Sweep only) for handler tests.
type fakeCleanup struct{ swept int }

func (f *fakeCleanup) Sweep(context.Context) (int, error) {
	f.swept++
	return 2, nil
}

func newAdminEnv(t *testing.T) (*httptest.Server, *fakeCleanup) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(ctx, "admin", "adminpw", "", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateUser(ctx, "bob", "bobpw", "", "", false); err != nil {
		t.Fatal(err)
	}
	settings, _ := core.NewSettingsService(filepath.Join(t.TempDir(), "configuration.yaml"), "", "", testutil.NewLogger())
	fc := &fakeCleanup{}
	h := NewHandler(Deps{Auth: auth, Users: store.Users, Settings: settings, Cleanup: fc, Logger: testutil.NewLogger()})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, fc
}

func TestCleanupRequiresAdmin(t *testing.T) {
	srv, _ := newAdminEnv(t)
	bob := login(t, srv, "bob")
	status, body := doMap(t, srv, http.MethodGet, "/admin/cleanup", bob, nil)
	if status != http.StatusForbidden {
		t.Fatalf("non-admin should get 403, got %d", status)
	}
	if errObj, _ := body["error"].(map[string]any); errObj["code"] != "forbidden" {
		t.Fatalf("expected forbidden error, got %+v", body)
	}
}

func TestCleanupToggleAndStatus(t *testing.T) {
	srv, _ := newAdminEnv(t)
	admin := login(t, srv, "admin")

	// Status: admin reads the current sweep state directly.
	status, body := doMap(t, srv, http.MethodGet, "/admin/cleanup", admin, nil)
	if status != http.StatusOK {
		t.Fatalf("status read failed: %d %+v", status, body)
	}
	if body["maxAgeSeconds"].(float64) != 720*3600 {
		t.Fatalf("unexpected maxAgeSeconds: %v", body["maxAgeSeconds"])
	}

	// Default is on (30-day window). Disabling persists; re-enabling persists.
	status, body = doMap(t, srv, http.MethodPut, "/admin/cleanup", admin, map[string]any{"enabled": false})
	if status != http.StatusOK || body["enabled"] != false {
		t.Fatalf("disable failed: %d %+v", status, body)
	}
	if body["maxAgeSeconds"].(float64) != 720*3600 {
		t.Fatalf("unexpected maxAgeSeconds: %v", body["maxAgeSeconds"])
	}

	status, body = doMap(t, srv, http.MethodPut, "/admin/cleanup", admin, map[string]any{"enabled": true})
	if status != http.StatusOK || body["enabled"] != true {
		t.Fatalf("enable failed: %d %+v", status, body)
	}
}

func TestCleanupRunNow(t *testing.T) {
	srv, fc := newAdminEnv(t)
	admin := login(t, srv, "admin")
	status, body := doMap(t, srv, http.MethodPost, "/admin/cleanup/runs", admin, nil)
	if status != http.StatusCreated {
		t.Fatalf("run failed: %d %+v", status, body)
	}
	if body["removed"].(float64) != 2 {
		t.Fatalf("expected removed=2, got %v", body["removed"])
	}
	if fc.swept != 1 {
		t.Fatalf("expected 1 sweep, got %d", fc.swept)
	}
}
