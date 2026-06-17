package core

import (
	"context"
	"errors"
	"testing"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/providers"
	"github.com/immerle/immerle/internal/providers/httpprovider"
	"github.com/immerle/immerle/internal/testutil"
)

func newManager(t *testing.T, builtins ...BuiltinDef) (*ProviderManager, *ProviderRegistry, *persistence.Store) {
	t.Helper()
	store := testutil.NewStore(t)
	reg := NewProviderRegistry()
	build := func(c models.ProviderConfig) (providers.Provider, error) {
		if c.Builtin() {
			return &fakeProvider{name: c.Name}, nil
		}
		return httpprovider.New(c.Name, c.Endpoint, c.Config)
	}
	mgr := NewProviderManager(store.ProviderConfigs, reg, build, builtins, testutil.NewLogger())
	return mgr, reg, store
}

func TestProviderManagerUpsertRegistersWhenEnabled(t *testing.T) {
	mgr, reg, _ := newManager(t)
	ctx := context.Background()

	saved, err := mgr.Upsert(ctx, models.ProviderConfig{
		Name: "manual", Endpoint: "https://svc.internal", Config: `{"quality":"hi"}`, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Kind != "http" || !saved.Enabled {
		t.Fatalf("unexpected saved config: %+v", saved)
	}
	if _, ok := reg.Get("manual"); !ok {
		t.Fatal("enabled provider should be live in the registry")
	}

	// Disable via upsert → unregistered but still persisted.
	if _, err := mgr.Upsert(ctx, models.ProviderConfig{Name: "manual", Endpoint: "https://svc.internal", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("manual"); ok {
		t.Fatal("disabled provider should be removed from the registry")
	}
	list, _ := mgr.List(ctx)
	if len(list) != 1 {
		t.Fatalf("config should persist while disabled, got %d", len(list))
	}
}

func TestProviderManagerValidation(t *testing.T) {
	mgr, _, _ := newManager(t)
	ctx := context.Background()

	cases := []models.ProviderConfig{
		{Name: "Bad Name", Endpoint: "https://x", Enabled: true},               // invalid slug
		{Name: "manual", Endpoint: "ftp://x", Enabled: true},                   // bad endpoint
		{Name: "manual", Endpoint: "https://x", Config: "{bad", Enabled: true}, // bad JSON
	}
	for i, c := range cases {
		if _, err := mgr.Upsert(ctx, c); err == nil {
			t.Fatalf("case %d should have failed: %+v", i, c)
		}
	}
}

func TestProviderManagerSetEnabledAndDelete(t *testing.T) {
	mgr, reg, _ := newManager(t)
	ctx := context.Background()

	if _, err := mgr.Upsert(ctx, models.ProviderConfig{Name: "manual", Endpoint: "https://svc", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("manual"); ok {
		t.Fatal("should start disabled")
	}

	if _, err := mgr.SetEnabled(ctx, "manual", true); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("manual"); !ok {
		t.Fatal("SetEnabled(true) should register")
	}

	if _, err := mgr.SetEnabled(ctx, "missing", true); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if err := mgr.Delete(ctx, "manual"); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("manual"); ok {
		t.Fatal("Delete should unregister")
	}
	if err := mgr.Delete(ctx, "manual"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("deleting twice should be ErrNotFound, got %v", err)
	}
}

func TestProviderManagerLoadRegistersEnabledOnly(t *testing.T) {
	mgr, reg, store := newManager(t)
	ctx := context.Background()

	// Persist directly: one enabled, one disabled.
	_ = store.ProviderConfigs.Upsert(ctx, models.ProviderConfig{Name: "on", Kind: "http", Endpoint: "https://a", Config: "{}", Enabled: true})
	_ = store.ProviderConfigs.Upsert(ctx, models.ProviderConfig{Name: "off", Kind: "http", Endpoint: "https://b", Config: "{}", Enabled: false})

	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("on"); !ok {
		t.Fatal("enabled provider should be loaded")
	}
	if _, ok := reg.Get("off"); ok {
		t.Fatal("disabled provider should not be loaded")
	}
}

func TestProviderManagerBuiltinsListedDisableNotDelete(t *testing.T) {
	mgr, reg, _ := newManager(t, BuiltinDef{Name: "jamendo", DefaultEnabled: true})
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// The built-in is listed, enabled by default, and live.
	list, _ := mgr.List(ctx)
	if len(list) != 1 || list[0].Name != "jamendo" || !list[0].Builtin() || !list[0].Enabled {
		t.Fatalf("built-in should be listed/enabled: %+v", list)
	}
	if _, ok := reg.Get("jamendo"); !ok {
		t.Fatal("enabled built-in should be live")
	}

	// It can be disabled (removed from the registry) but stays persisted.
	if _, err := mgr.SetEnabled(ctx, "jamendo", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("jamendo"); ok {
		t.Fatal("disabled built-in should be unregistered")
	}
	// Re-enabling brings back the original instance.
	if _, err := mgr.SetEnabled(ctx, "jamendo", true); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("jamendo"); !ok {
		t.Fatal("re-enabled built-in should be live again")
	}

	// It cannot be deleted.
	if err := mgr.Delete(ctx, "jamendo"); err == nil {
		t.Fatal("deleting a built-in should fail")
	}
}

func TestProviderManagerReorder(t *testing.T) {
	mgr, reg, _ := newManager(t, BuiltinDef{Name: "jamendo", DefaultEnabled: true})
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Upsert(ctx, models.ProviderConfig{Name: "manual", Endpoint: "https://svc", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	// A newly added provider goes to the front → it sorts before the built-in.
	if got := registryNames(reg); got[0] != "manual" {
		t.Fatalf("expected newly added 'manual' first, got %v", got)
	}

	// Put the built-in first via an explicit reorder.
	if err := mgr.Reorder(ctx, []string{"jamendo", "manual"}); err != nil {
		t.Fatal(err)
	}
	if got := registryNames(reg); got[0] != "jamendo" || got[1] != "manual" {
		t.Fatalf("reorder not applied to registry: %v", got)
	}
	list, _ := mgr.List(ctx)
	if list[0].Name != "jamendo" || list[1].Name != "manual" {
		t.Fatalf("reorder not persisted: %+v", list)
	}

	// Incomplete or unknown orders are rejected.
	if err := mgr.Reorder(ctx, []string{"manual"}); err == nil {
		t.Fatal("incomplete reorder should fail")
	}
	if err := mgr.Reorder(ctx, []string{"manual", "ghost"}); err == nil {
		t.Fatal("unknown name should fail")
	}
}

func TestProviderManagerNewProvidersGoFirst(t *testing.T) {
	mgr, reg, _ := newManager(t, BuiltinDef{Name: "jamendo", DefaultEnabled: true})
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// Each newly added provider lands ahead of every existing one.
	if _, err := mgr.Upsert(ctx, models.ProviderConfig{Name: "first", Endpoint: "https://a", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Upsert(ctx, models.ProviderConfig{Name: "second", Endpoint: "https://b", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// Most-recently-added is first; the built-in seeded at load stays last.
	if got := registryNames(reg); got[0] != "second" || got[1] != "first" || got[2] != "jamendo" {
		t.Fatalf("expected newest-first order [second first jamendo], got %v", got)
	}

	// Editing an existing provider keeps its position (does not jump to front).
	if _, err := mgr.Upsert(ctx, models.ProviderConfig{Name: "first", Endpoint: "https://a2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if got := registryNames(reg); got[0] != "second" || got[1] != "first" {
		t.Fatalf("edit must preserve order, got %v", got)
	}
}

func TestProviderManagerLoadPrunesStaleBuiltins(t *testing.T) {
	mgr, reg, store := newManager(t, BuiltinDef{Name: "internet-archive", DefaultEnabled: true})
	ctx := context.Background()

	// Simulate rows left by older releases: built-ins that no longer exist, plus
	// a user-created dynamic provider that must survive.
	_ = store.ProviderConfigs.Upsert(ctx, models.ProviderConfig{Name: "sample", Kind: "builtin", Enabled: true, SortOrder: 1})
	_ = store.ProviderConfigs.Upsert(ctx, models.ProviderConfig{Name: "internetarchive", Kind: "builtin", Enabled: true, SortOrder: 2})
	_ = store.ProviderConfigs.Upsert(ctx, models.ProviderConfig{Name: "deezer", Kind: "http", Endpoint: "https://deezer-http", Config: "{}", Enabled: true, SortOrder: 3})

	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	list, _ := mgr.List(ctx)
	for _, p := range list {
		got[p.Name] = true
	}
	if got["sample"] || got["internetarchive"] {
		t.Fatalf("stale built-ins should be pruned, got %+v", got)
	}
	if !got["internet-archive"] {
		t.Fatal("current built-in should be present")
	}
	if !got["deezer"] {
		t.Fatal("dynamic provider must NOT be pruned")
	}
	if _, ok := reg.Get("internetarchive"); ok {
		t.Fatal("stale built-in should be unregistered")
	}
}

func registryNames(reg *ProviderRegistry) []string {
	all := reg.All()
	names := make([]string, len(all))
	for i, p := range all {
		names[i] = p.Name()
	}
	return names
}
