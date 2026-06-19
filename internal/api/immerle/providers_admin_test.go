package immerle

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/providers"
	"github.com/immerle/immerle/internal/providers/httpprovider"
	"github.com/immerle/immerle/internal/testutil"
)

// stubProvider stands in for a built provider so the CRUD/reorder tests don't
// hit the network: it is not a providers.Verifier, so the capabilities check on
// enable is skipped (that wiring is covered in the core package).
type stubProvider struct{ name string }

func (s stubProvider) Name() string { return s.name }
func (s stubProvider) Search(context.Context, string, int) ([]providers.Result, error) {
	return nil, nil
}
func (s stubProvider) Resolve(context.Context, string) (providers.Result, error) {
	return providers.Result{}, nil
}
func (s stubProvider) Download(context.Context, string, io.Writer) error { return nil }
func (s stubProvider) MaxQuality() string                                { return "remote" }

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
		if _, err := httpprovider.New(c.Name, c.Endpoint, c.Config); err != nil {
			return nil, err
		}
		return stubProvider{name: c.Name}, nil
	}
	mgr := core.NewProviderManager(store.ProviderConfigs, reg, build, nil, testutil.NewLogger())
	h := NewHandler(Deps{Auth: auth, Users: store.Users, Providers: mgr, Logger: testutil.NewLogger()})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestProvidersAdminOnly(t *testing.T) {
	srv := newProvidersEnv(t)
	bob := login(t, srv, "bob")
	status, body := doMap(t, srv, http.MethodPost, "/admin/providers", bob, map[string]any{
		"name": "manual", "endpoint": "https://x",
	})
	if status != http.StatusForbidden {
		t.Fatalf("non-admin should get 403, got %d", status)
	}
	if errObj, _ := body["error"].(map[string]any); errObj["code"] != "forbidden" {
		t.Fatalf("expected forbidden error, got %+v", body)
	}
}

func TestProvidersCrudFlow(t *testing.T) {
	srv := newProvidersEnv(t)
	admin := login(t, srv, "admin")

	// Create.
	status, prov := doMap(t, srv, http.MethodPost, "/admin/providers", admin, map[string]any{
		"name":     "manual",
		"endpoint": "https://svc.internal",
		"config":   `{"quality":"hi"}`,
	})
	if status != http.StatusOK {
		t.Fatalf("create failed: %d %+v", status, prov)
	}
	if prov["name"] != "manual" || prov["enabled"] != true || prov["active"] != true {
		t.Fatalf("create failed: %+v", prov)
	}

	// List shows it.
	st, provs := doArr(t, srv, http.MethodGet, "/admin/providers", admin, nil)
	if st != http.StatusOK || len(provs) != 1 {
		t.Fatalf("expected 1 provider, got status %d len %d", st, len(provs))
	}

	// Disable → no longer active but still listed.
	status, prov = doMap(t, srv, http.MethodPut, "/admin/providers/manual/enabled", admin, map[string]any{"enabled": false})
	if status != http.StatusOK {
		t.Fatalf("disable failed: %d %+v", status, prov)
	}
	if prov["enabled"] != false || prov["active"] != false {
		t.Fatalf("disable failed: %+v", prov)
	}

	// Delete.
	if st := doStatus(t, srv, http.MethodDelete, "/admin/providers/manual", admin, nil); st != http.StatusNoContent {
		t.Fatalf("delete failed: %d", st)
	}
	st, provs = doArr(t, srv, http.MethodGet, "/admin/providers", admin, nil)
	if st != http.StatusOK || len(provs) != 0 {
		t.Fatalf("expected 0 providers after delete, got status %d len %d", st, len(provs))
	}
}

func TestProvidersRejectsBadConfig(t *testing.T) {
	srv := newProvidersEnv(t)
	admin := login(t, srv, "admin")
	status, body := doMap(t, srv, http.MethodPost, "/admin/providers", admin, map[string]any{
		"name":     "manual",
		"endpoint": "https://x",
		"config":   "{not json",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("bad config should be 400, got %d", status)
	}
	if errObj, _ := body["error"].(map[string]any); errObj["code"] != "provider_invalid_config" {
		t.Fatalf("expected provider_invalid_config error, got %+v", body)
	}
}

func TestProvidersBuiltinAndReorder(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	_, _ = auth.CreateUser(ctx, "admin", "adminpw", "", "", true)
	reg := core.NewProviderRegistry()
	build := func(c models.ProviderConfig) (providers.Provider, error) {
		endpoint := c.Endpoint
		if c.Builtin() {
			endpoint = "https://example.test" // stand-in instance
		}
		if _, err := httpprovider.New(c.Name, endpoint, c.Config); err != nil {
			return nil, err
		}
		return stubProvider{name: c.Name}, nil
	}
	// A built-in provider declared with a default config + enabled state.
	mgr := core.NewProviderManager(store.ProviderConfigs, reg, build,
		[]core.BuiltinDef{{Name: "jamendo", DefaultEnabled: true}}, testutil.NewLogger())
	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Deps{Auth: auth, Users: store.Users, Providers: mgr, Logger: testutil.NewLogger()})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	admin := login(t, srv, "admin")

	// Add a dynamic provider.
	if st := doStatus(t, srv, http.MethodPost, "/admin/providers", admin, map[string]any{
		"name":     "manual",
		"endpoint": "https://svc.internal",
	}); st != http.StatusOK {
		t.Fatalf("add dynamic provider failed: %d", st)
	}

	// List shows the built-in (not deletable) and the dynamic (deletable).
	st, provs := doArr(t, srv, http.MethodGet, "/admin/providers", admin, nil)
	if st != http.StatusOK || len(provs) != 2 {
		t.Fatalf("expected 2 providers, got status %d len %d", st, len(provs))
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
	if st := doStatus(t, srv, http.MethodDelete, "/admin/providers/jamendo", admin, nil); st != http.StatusBadRequest {
		t.Fatalf("deleting a built-in should be 400, got %d", st)
	}

	// Reorder: put the dynamic provider first.
	st, provs = doArr(t, srv, http.MethodPut, "/admin/providers/order", admin, map[string]any{
		"order": []string{"manual", "jamendo"},
	})
	if st != http.StatusOK {
		t.Fatalf("reorder failed: %d", st)
	}
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
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	admin := login(t, srv, "admin")

	status, body := doMap(t, srv, http.MethodPost, "/admin/providers", admin, map[string]any{"name": "manual"})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when subsystem disabled, got %d", status)
	}
	if errObj, _ := body["error"].(map[string]any); errObj["code"] != "unavailable" {
		t.Fatalf("expected unavailable error, got %+v", body)
	}
}
