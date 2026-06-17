package immerle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/providers"
	"github.com/immerle/immerle/internal/providers/httpprovider"
	"github.com/immerle/immerle/internal/testutil"
)

func newProvidersEnv(t *testing.T) *httptest.Server {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(ctx, "admin", "adminpw", "", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateUser(ctx, "bob", "bobpw", "", "", false); err != nil {
		t.Fatal(err)
	}
	reg := core.NewProviderRegistry()
	build := func(c models.ProviderConfig) (providers.Provider, error) {
		return httpprovider.New(c.Name, c.Endpoint, c.Config)
	}
	mgr := core.NewProviderManager(store.ProviderConfigs, reg, build, nil, testutil.NewLogger())
	h := NewHandler(Deps{Auth: auth, Users: store.Users, Providers: mgr, Logger: testutil.NewLogger()})
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func admin() url.Values { return url.Values{"u": {"admin"}, "p": {"adminpw"}, "c": {"test"}} }

func TestProvidersAdminOnly(t *testing.T) {
	srv := newProvidersEnv(t)
	resp, err := http.PostForm(srv.URL+"/admin/providers", url.Values{
		"u": {"bob"}, "p": {"bobpw"}, "c": {"test"}, "name": {"manual"}, "endpoint": {"https://x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin should get 403, got %d", resp.StatusCode)
	}
}

func TestProvidersCrudFlow(t *testing.T) {
	srv := newProvidersEnv(t)

	// Create.
	v := admin()
	v.Set("name", "manual")
	v.Set("endpoint", "https://svc.internal")
	v.Set("config", `{"quality":"hi"}`)
	body := postForm(t, srv, "/admin/providers", v)
	prov, _ := body["provider"].(map[string]any)
	if prov["name"] != "manual" || prov["enabled"] != true || prov["active"] != true {
		t.Fatalf("create failed: %+v", body)
	}

	// List shows it.
	body = postFormGet(t, srv, "/admin/providers", admin())
	provs, _ := body["providers"].([]any)
	if len(provs) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(provs))
	}

	// Disable → no longer active but still listed.
	dv := admin()
	dv.Set("name", "manual")
	dv.Set("enabled", "false")
	body = postForm(t, srv, "/admin/providers/enable", dv)
	prov, _ = body["provider"].(map[string]any)
	if prov["enabled"] != false || prov["active"] != false {
		t.Fatalf("disable failed: %+v", body)
	}

	// Delete.
	delv := admin()
	delv.Set("name", "manual")
	if body = postForm(t, srv, "/admin/providers/delete", delv); body["ok"] != true {
		t.Fatalf("delete failed: %+v", body)
	}
	body = postFormGet(t, srv, "/admin/providers", admin())
	if provs, _ := body["providers"].([]any); len(provs) != 0 {
		t.Fatalf("expected 0 providers after delete, got %d", len(provs))
	}
}

func TestProvidersRejectsBadConfig(t *testing.T) {
	srv := newProvidersEnv(t)
	v := admin()
	v.Set("name", "manual")
	v.Set("endpoint", "https://x")
	v.Set("config", "{not json")
	resp, err := http.PostForm(srv.URL+"/admin/providers", v)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad config should be 400, got %d", resp.StatusCode)
	}
}

func TestProvidersBuiltinAndReorder(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	_, _ = auth.CreateUser(ctx, "admin", "adminpw", "", "", true)
	reg := core.NewProviderRegistry()
	build := func(c models.ProviderConfig) (providers.Provider, error) {
		if c.Builtin() {
			return httpprovider.New(c.Name, "https://example.test", "{}") // stand-in instance
		}
		return httpprovider.New(c.Name, c.Endpoint, c.Config)
	}
	// A built-in provider declared with a default config + enabled state.
	mgr := core.NewProviderManager(store.ProviderConfigs, reg, build,
		[]core.BuiltinDef{{Name: "jamendo", DefaultEnabled: true}}, testutil.NewLogger())
	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Deps{Auth: auth, Users: store.Users, Providers: mgr, Logger: testutil.NewLogger()})
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Add a dynamic provider.
	v := admin()
	v.Set("name", "manual")
	v.Set("endpoint", "https://svc.internal")
	postForm(t, srv, "/admin/providers", v)

	// List shows the built-in (not deletable) and the dynamic (deletable).
	body := postFormGet(t, srv, "/admin/providers", admin())
	provs, _ := body["providers"].([]any)
	if len(provs) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(provs))
	}
	byName := map[string]map[string]any{}
	for _, p := range provs {
		m := p.(map[string]any)
		byName[m["name"].(string)] = m
	}
	if byName["jamendo"]["builtin"] != true || byName["jamendo"]["deletable"] != false {
		t.Fatalf("built-in flags wrong: %+v", byName["jamendo"])
	}
	if byName["manual"]["deletable"] != true {
		t.Fatalf("dynamic should be deletable: %+v", byName["manual"])
	}

	// A built-in cannot be deleted.
	dv := admin()
	dv.Set("name", "jamendo")
	resp, _ := http.PostForm(srv.URL+"/admin/providers/delete", dv)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("deleting a built-in should be 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Reorder: put the dynamic provider first.
	rv := admin()
	rv.Set("order", "manual,jamendo")
	body = postForm(t, srv, "/admin/providers/reorder", rv)
	provs, _ = body["providers"].([]any)
	if provs[0].(map[string]any)["name"] != "manual" {
		t.Fatalf("reorder not reflected: %+v", provs)
	}
}

func TestProvidersDisabledSubsystem(t *testing.T) {
	// No Providers controller → 503.
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	_, _ = auth.CreateUser(ctx, "admin", "adminpw", "", "", true)
	h := NewHandler(Deps{Auth: auth, Users: store.Users, Logger: testutil.NewLogger()})
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.PostForm(srv.URL+"/admin/providers", admin())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when subsystem disabled, got %d", resp.StatusCode)
	}
}

// postFormGet issues a GET with query params and decodes the JSON body.
func postFormGet(t *testing.T, srv *httptest.Server, path string, v url.Values) map[string]any {
	t.Helper()
	resp, err := http.Get(srv.URL + path + "?" + v.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}
