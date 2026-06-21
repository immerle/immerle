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

func newSettingsEnv(t *testing.T) *httptest.Server {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	_, _ = auth.CreateUser(ctx, "admin", "adminpw", "", "", true)
	_, _ = auth.CreateUser(ctx, "bob", "bobpw", "", "", false)
	settings, err := core.NewSettingsService(filepath.Join(t.TempDir(), "configuration.yaml"), "", "", testutil.NewLogger())
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Deps{Auth: auth, Users: store.Users, Settings: settings, Logger: testutil.NewLogger()})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSettingsAdminOnly(t *testing.T) {
	srv := newSettingsEnv(t)
	bob := login(t, srv, "bob")
	status, body := doMap(t, srv, http.MethodGet, "/admin/settings", bob, nil)
	if status != http.StatusForbidden {
		t.Fatalf("non-admin should get 403, got %d", status)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "forbidden" {
		t.Fatalf("expected forbidden error code, got %+v", body)
	}
}

func TestSettingsGetAndHotUpdate(t *testing.T) {
	srv := newSettingsEnv(t)
	admin := login(t, srv, "admin")

	_, body := doMap(t, srv, http.MethodGet, "/admin/settings", admin, nil)
	if body["restartRequired"] != false {
		t.Fatalf("nothing should require restart initially: %+v", body)
	}

	// Hot change: provider behaviour → applied immediately, no restart.
	_, out := doMap(t, srv, http.MethodPatch, "/admin/settings", admin, map[string]any{
		"providers": map[string]any{"autoDownloadOnPlay": false, "searchTimeoutSeconds": 10},
	})
	if out["restartRequired"] != false {
		t.Fatalf("provider-behaviour change should be hot: %+v", out)
	}
	settings, _ := out["settings"].(map[string]any)
	provs, _ := settings["providers"].(map[string]any)
	if provs["autoDownloadOnPlay"] != false || provs["searchTimeoutSeconds"].(float64) != 10 {
		t.Fatalf("settings not applied: %+v", provs)
	}
}

func TestSettingsRestartRequired(t *testing.T) {
	srv := newSettingsEnv(t)
	admin := login(t, srv, "admin")

	// Toggling the scan watcher is restart-required (the watcher is wired at boot).
	_, out := doMap(t, srv, http.MethodPatch, "/admin/settings", admin, map[string]any{
		"scan": map[string]any{"watch": false},
	})
	if out["restartRequired"] != true {
		t.Fatalf("toggling the scan watcher should require a restart: %+v", out)
	}
	pending, _ := out["pendingRestart"].([]any)
	found := false
	for _, p := range pending {
		if p == "scan.watch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("scan.watch should be in pendingRestart: %+v", pending)
	}
}

func TestFederationIsHotReloadable(t *testing.T) {
	srv := newSettingsEnv(t)
	admin := login(t, srv, "admin")

	_, out := doMap(t, srv, http.MethodPatch, "/admin/settings", admin, map[string]any{
		"federation": map[string]any{"userId": "6f1c2b8e-1f0a-4f9b-9c3a-1e2d3c4b5a6f"},
	})
	if out["restartRequired"] != false {
		t.Fatalf("federation changes should be hot (no restart): %+v", out)
	}
}
