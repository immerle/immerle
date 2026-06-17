package immerle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

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
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, fc
}

func TestCleanupRequiresAdmin(t *testing.T) {
	srv, _ := newAdminEnv(t)
	resp, err := http.PostForm(srv.URL+"/admin/cleanup", url.Values{
		"u": {"bob"}, "p": {"bobpw"}, "c": {"test"}, "enabled": {"true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin should get 403, got %d", resp.StatusCode)
	}
}

func TestCleanupToggleAndStatus(t *testing.T) {
	srv, _ := newAdminEnv(t)
	admin := func() url.Values { return url.Values{"u": {"admin"}, "p": {"adminpw"}, "c": {"test"}} }

	resp, _ := http.PostForm(srv.URL+"/admin/cleanup", admin()) // POST without enabled → 400
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST without enabled should be 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Default is on (30-day window). Disabling persists; re-enabling persists.
	v := admin()
	v.Set("enabled", "false")
	body := postForm(t, srv, "/admin/cleanup", v)
	if body["ok"] != true || body["enabled"] != false {
		t.Fatalf("disable failed: %+v", body)
	}
	if body["maxAgeSeconds"].(float64) != 720*3600 {
		t.Fatalf("unexpected maxAgeSeconds: %v", body["maxAgeSeconds"])
	}

	v.Set("enabled", "true")
	body = postForm(t, srv, "/admin/cleanup", v)
	if body["enabled"] != true {
		t.Fatalf("enable failed: %+v", body)
	}
}

func TestCleanupRunNow(t *testing.T) {
	srv, fc := newAdminEnv(t)
	body := postForm(t, srv, "/admin/cleanup/run", url.Values{"u": {"admin"}, "p": {"adminpw"}, "c": {"test"}})
	if body["ok"] != true {
		t.Fatalf("run failed: %+v", body)
	}
	if body["removed"].(float64) != 2 {
		t.Fatalf("expected removed=2, got %v", body["removed"])
	}
	if fc.swept != 1 {
		t.Fatalf("expected 1 sweep, got %d", fc.swept)
	}
}
