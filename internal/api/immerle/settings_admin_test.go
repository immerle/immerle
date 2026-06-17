package immerle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/testutil"
)

func decode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

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
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSettingsAdminOnly(t *testing.T) {
	srv := newSettingsEnv(t)
	resp, err := http.Get(srv.URL + "/admin/settings?u=bob&p=bobpw&c=test")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin should get 403, got %d", resp.StatusCode)
	}
}

func TestSettingsGetAndHotUpdate(t *testing.T) {
	srv := newSettingsEnv(t)

	body := postFormGet(t, srv, "/admin/settings", admin())
	if body["restartRequired"] != false {
		t.Fatalf("nothing should require restart initially: %+v", body)
	}

	// Hot change: provider behaviour → applied immediately, no restart.
	resp, err := http.Post(
		srv.URL+"/admin/settings?u=admin&p=adminpw&c=test",
		"application/json",
		strings.NewReader(`{"providers":{"autoDownloadOnPlay":false,"searchTimeoutSeconds":10}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := decode(t, resp)
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

	// Toggling the scan watcher is restart-required (the watcher is wired at boot).
	resp, err := http.Post(
		srv.URL+"/admin/settings?u=admin&p=adminpw&c=test",
		"application/json",
		strings.NewReader(`{"scan":{"watch":false}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := decode(t, resp)
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

	resp, err := http.Post(
		srv.URL+"/admin/settings?u=admin&p=adminpw&c=test",
		"application/json",
		strings.NewReader(`{"federation":{"enabled":true,"hubUrl":"https://hub.test"}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := decode(t, resp)
	if out["restartRequired"] != false {
		t.Fatalf("federation changes should be hot (no restart): %+v", out)
	}
}
