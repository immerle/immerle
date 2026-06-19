package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/providers"
)

// providerNameRe constrains dynamic provider names to a simple slug so they are
// safe to use in ids, paths and config keys.
var providerNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// ProviderBuilder constructs a live Provider from a stored config. Injected so
// core stays decoupled from concrete provider implementations. It must handle
// both kinds: "builtin" (a compiled-in factory, credentials taken from the
// config JSON) and "http" (a dynamic content-neutral HTTP provider).
type ProviderBuilder func(cfg models.ProviderConfig) (providers.Provider, error)

// BuiltinDef declares a compiled-in provider that is managed via the admin API:
// its credentials/options live in the config JSON (editable in the UI), and it
// is seeded with a default config and enabled state on first run.
type BuiltinDef struct {
	Name string
	// DefaultConfig is the JSON config the provider's row is seeded with (e.g.
	// a template with a credential placeholder).
	DefaultConfig string
	// DefaultEnabled is the seeded enabled state.
	DefaultEnabled bool
}

// ProviderManager owns the configurable on-demand providers — both the dynamic
// HTTP ones and the built-in (compiled-in factory) ones. It is the single writer
// to the persisted configs and the live registry, so the two never drift.
//
// Built-in providers are surfaced alongside dynamic ones and configured the same
// way (a JSON config edited via the admin API); they can be disabled and
// reordered, but not deleted. Their credentials live entirely in the config —
// nothing comes from the environment.
type ProviderManager struct {
	// mu serializes the public mutating methods: each does a read-modify-write
	// across the persisted configs and the live registry, and concurrent admin
	// mutations would otherwise desync registry order from persisted SortOrder.
	// ponytail: one coarse lock; admin mutations are rare, so contention is a
	// non-issue.
	mu           sync.Mutex
	repo         *persistence.ProviderConfigRepo
	registry     *ProviderRegistry
	build        ProviderBuilder
	builtins     map[string]BuiltinDef
	builtinOrder []string // stable order built-ins were declared in
	logger       *slog.Logger
}

// NewProviderManager builds a manager. builtins declares the compiled-in
// providers (seeded + non-deletable).
func NewProviderManager(repo *persistence.ProviderConfigRepo, registry *ProviderRegistry, build ProviderBuilder, builtins []BuiltinDef, logger *slog.Logger) *ProviderManager {
	m := &ProviderManager{
		repo:     repo,
		registry: registry,
		build:    build,
		builtins: make(map[string]BuiltinDef, len(builtins)),
		logger:   logger,
	}
	for _, b := range builtins {
		if _, dup := m.builtins[b.Name]; dup {
			continue
		}
		m.builtins[b.Name] = b
		m.builtinOrder = append(m.builtinOrder, b.Name)
	}
	return m
}

// Load reconciles persisted configs with the built-ins and registers every
// enabled provider into the live registry, in sort order. Call once at startup.
func (m *ProviderManager) Load(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureBuiltins(ctx); err != nil {
		return err
	}
	configs, err := m.repo.List(ctx)
	if err != nil {
		return err
	}
	var order []string
	for _, cfg := range configs {
		order = append(order, cfg.Name)
		if !cfg.Enabled {
			continue
		}
		if err := m.activate(cfg); err != nil {
			m.logger.Warn("provider failed to load", "provider", cfg.Name, "error", err)
		}
	}
	m.registry.Reorder(order)
	return nil
}

// ensureBuiltins reconciles the persisted built-in rows with the providers
// actually compiled in: it prunes built-in rows that no longer match a declared
// built-in (one removed or renamed between releases) and seeds a row — with its
// default config and enabled state — for each new built-in.
func (m *ProviderManager) ensureBuiltins(ctx context.Context) error {
	existing, err := m.repo.List(ctx)
	if err != nil {
		return err
	}
	have := make(map[string]bool, len(existing))
	maxOrder := 0
	for _, c := range existing {
		if c.Builtin() {
			if _, ok := m.builtins[c.Name]; !ok {
				// Stale built-in (e.g. old "sample"/"internetarchive" name).
				if err := m.repo.Delete(ctx, c.Name); err != nil {
					return err
				}
				m.registry.Unregister(c.Name)
				m.logger.Info("pruned stale built-in provider", "provider", c.Name)
				continue
			}
		}
		have[c.Name] = true
		if c.SortOrder > maxOrder {
			maxOrder = c.SortOrder
		}
	}
	for _, name := range m.builtinOrder {
		if have[name] {
			continue
		}
		def := m.builtins[name]
		cfg := def.DefaultConfig
		if strings.TrimSpace(cfg) == "" {
			cfg = "{}"
		}
		maxOrder++
		if err := m.repo.Upsert(ctx, models.ProviderConfig{
			Name: name, Kind: "builtin", Config: cfg, Enabled: def.DefaultEnabled, SortOrder: maxOrder,
		}); err != nil {
			return err
		}
	}
	return nil
}

// List returns all persisted provider configs (built-in and dynamic), ordered.
func (m *ProviderManager) List(ctx context.Context) ([]models.ProviderConfig, error) {
	return m.repo.List(ctx)
}

// Active reports whether a provider is currently live in the registry.
func (m *ProviderManager) Active(name string) bool {
	_, ok := m.registry.Get(name)
	return ok
}

// Upsert validates and persists a provider config, then (un)registers it to
// match its enabled flag. For a built-in only the config + enabled flag are
// honoured (kind/endpoint are fixed); a built-in's credentials are edited here.
func (m *ProviderManager) Upsert(ctx context.Context, cfg models.ProviderConfig) (models.ProviderConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg.Name = strings.TrimSpace(cfg.Name)
	if !providerNameRe.MatchString(cfg.Name) {
		return cfg, fmt.Errorf("invalid provider name (use lowercase letters, digits, '-' or '_')")
	}

	if _, isBuiltin := m.builtins[cfg.Name]; isBuiltin {
		cfg.Kind = "builtin"
		cfg.Endpoint = ""
	} else {
		if cfg.Kind == "" {
			cfg.Kind = "http"
		}
		if cfg.Kind != "http" {
			return cfg, fmt.Errorf("unsupported provider kind %q (only \"http\")", cfg.Kind)
		}
	}
	if strings.TrimSpace(cfg.Config) == "" {
		cfg.Config = "{}"
	}
	// Enforce the unified config schema ({ header, params, … }); rejects invalid
	// JSON and unknown keys with a clear message before anything is persisted.
	if _, err := providers.ParseConfig(cfg.Config); err != nil {
		return cfg, err
	}

	// Preserve sort order on update; a newly created provider goes to the front
	// (highest priority) so it is used first without a manual reorder.
	if existing, err := m.repo.Get(ctx, cfg.Name); err == nil {
		cfg.SortOrder = existing.SortOrder
	} else {
		cfg.SortOrder = m.firstOrder(ctx)
	}

	// Build once up front so an invalid endpoint/config is rejected before we
	// persist it (fail fast, no half-applied state).
	built, err := m.build(cfg)
	if err != nil {
		return cfg, err
	}
	// On save, an HTTP provider's config must be coherent with its /capabilities
	// (the protocol matches and every required field is supplied). Built-ins have
	// no capabilities endpoint, so this is a no-op for them.
	if err := verifyProvider(ctx, built); err != nil {
		return cfg, err
	}
	if err := m.repo.Upsert(ctx, cfg); err != nil {
		return cfg, err
	}
	if cfg.Enabled {
		m.registry.Register(built)
		m.logger.Info("provider registered", "provider", cfg.Name, "kind", cfg.Kind)
	} else {
		m.registry.Unregister(cfg.Name)
	}
	m.reorderFromDB(ctx)
	return m.repo.Get(ctx, cfg.Name)
}

// SetEnabled toggles any provider (built-in or dynamic) on or off, updating the
// live registry to match.
func (m *ProviderManager) SetEnabled(ctx context.Context, name string, enabled bool) (models.ProviderConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.repo.Get(ctx, name)
	if err != nil {
		return cfg, err
	}
	if enabled {
		// Verify before persisting the enabled flag, so a provider whose
		// capabilities aren't satisfied stays disabled (nothing half-applied).
		built, err := m.build(cfg)
		if err != nil {
			return cfg, err
		}
		if err := verifyProvider(ctx, built); err != nil {
			return cfg, err
		}
		if err := m.repo.SetEnabled(ctx, name, true); err != nil {
			return cfg, err
		}
		cfg.Enabled = true
		m.registry.Register(built)
		m.reorderFromDB(ctx)
	} else {
		if err := m.repo.SetEnabled(ctx, name, false); err != nil {
			return cfg, err
		}
		cfg.Enabled = false
		m.registry.Unregister(name)
	}
	m.logger.Info("provider toggled", "provider", name, "enabled", enabled)
	return m.repo.Get(ctx, name)
}

// verifyProvider runs a provider's capability check if it implements Verifier
// (HTTP providers do; built-ins don't).
func verifyProvider(ctx context.Context, p providers.Provider) error {
	if v, ok := p.(providers.Verifier); ok {
		return v.Verify(ctx)
	}
	return nil
}

// CreateFromURL creates a dynamic HTTP provider from just its base URL. It
// probes the mandatory /capabilities endpoint to confirm the service exists and
// speaks the protocol, takes the provider name the remote declares, and seeds
// the config with the declared fields set to null (the admin fills them in
// later via the settings panel). The provider is created disabled.
func (m *ProviderManager) CreateFromURL(ctx context.Context, endpoint string) (models.ProviderConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	prov, err := m.build(models.ProviderConfig{Name: "probe", Kind: "http", Endpoint: endpoint, Config: "{}"})
	if err != nil {
		return models.ProviderConfig{}, err
	}
	cp, ok := prov.(providers.CapabilityProvider)
	if !ok {
		return models.ProviderConfig{}, fmt.Errorf("provider does not expose capabilities")
	}
	caps, err := cp.Capabilities(ctx)
	if err != nil {
		return models.ProviderConfig{}, err
	}
	if caps.Version != providers.ProtocolVersion {
		return models.ProviderConfig{}, fmt.Errorf("remote protocol version %d unsupported (expected %d)", caps.Version, providers.ProtocolVersion)
	}
	if !providerNameRe.MatchString(caps.Name) {
		return models.ProviderConfig{}, fmt.Errorf("capabilities name %q is not a valid slug", caps.Name)
	}
	if _, err := m.repo.Get(ctx, caps.Name); err == nil {
		return models.ProviderConfig{}, fmt.Errorf("provider %q already exists", caps.Name)
	}

	cfg := models.ProviderConfig{
		Name:      caps.Name,
		Kind:      "http",
		Endpoint:  strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		Config:    skeletonConfig(caps),
		Enabled:   false, // created disabled; the admin fills the config then enables
		SortOrder: m.firstOrder(ctx),
	}
	if err := m.repo.Upsert(ctx, cfg); err != nil {
		return models.ProviderConfig{}, err
	}
	m.reorderFromDB(ctx)
	m.logger.Info("provider created from url", "provider", cfg.Name, "endpoint", endpoint)
	return m.repo.Get(ctx, caps.Name)
}

// skeletonConfig builds a { header, params } config payload from a capabilities
// schema, with every declared field set to null for the admin to fill in.
func skeletonConfig(caps providers.Capabilities) string {
	header := map[string]any{}
	params := map[string]any{}
	for key, f := range caps.Config {
		switch f.Where {
		case "header":
			header[key] = nil
		case "params":
			params[key] = nil
		}
	}
	obj := map[string]any{}
	if len(header) > 0 {
		obj["header"] = header
	}
	if len(params) > 0 {
		obj["params"] = params
	}
	b, _ := json.Marshal(obj)
	return string(b)
}

// Versions fetches each dynamic (HTTP) provider's live protocol version in
// parallel, best-effort: unreachable providers are simply omitted. Time-bounded
// so the admin list stays responsive. Does not take the manager lock (read-only
// introspection, no network call while holding it).
func (m *ProviderManager) Versions(ctx context.Context) map[string]int {
	configs, err := m.repo.List(ctx)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	out := map[string]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, c := range configs {
		if c.Builtin() {
			continue
		}
		wg.Add(1)
		go func(c models.ProviderConfig) {
			defer wg.Done()
			prov, err := m.build(c)
			if err != nil {
				return
			}
			cp, ok := prov.(providers.CapabilityProvider)
			if !ok {
				return
			}
			caps, err := cp.Capabilities(ctx)
			if err != nil {
				return
			}
			mu.Lock()
			out[c.Name] = caps.Version
			mu.Unlock()
		}(c)
	}
	wg.Wait()
	return out
}

// Reorder sets the provider priority to the given name order. Every persisted
// provider must appear exactly once. The live registry is updated to match.
func (m *ProviderManager) Reorder(ctx context.Context, names []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	configs, err := m.repo.List(ctx)
	if err != nil {
		return err
	}
	if len(names) != len(configs) {
		return fmt.Errorf("reorder must list all %d providers exactly once", len(configs))
	}
	known := make(map[string]bool, len(configs))
	for _, c := range configs {
		known[c.Name] = true
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if !known[n] {
			return fmt.Errorf("unknown provider %q", n)
		}
		if seen[n] {
			return fmt.Errorf("provider %q listed twice", n)
		}
		seen[n] = true
	}
	for i, n := range names {
		if err := m.repo.SetOrder(ctx, n, i+1); err != nil {
			return err
		}
	}
	m.registry.Reorder(names)
	return nil
}

// Delete removes a dynamic provider and unregisters it. Built-in providers
// cannot be deleted (disable them instead).
func (m *ProviderManager) Delete(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, isBuiltin := m.builtins[name]; isBuiltin {
		return fmt.Errorf("provider %q is built-in and cannot be deleted (disable it instead)", name)
	}
	if _, err := m.repo.Get(ctx, name); err != nil {
		return err
	}
	if err := m.repo.Delete(ctx, name); err != nil {
		return err
	}
	m.registry.Unregister(name)
	m.logger.Info("dynamic provider deleted", "provider", name)
	return nil
}

// activate builds cfg (built-in via factory from its config, or dynamic http)
// and registers it.
func (m *ProviderManager) activate(cfg models.ProviderConfig) error {
	p, err := m.build(cfg)
	if err != nil {
		return err
	}
	m.registry.Register(p)
	return nil
}

// firstOrder returns one below the current minimum sort order, so a newly added
// provider sorts ahead of every existing one (front of the priority list).
func (m *ProviderManager) firstOrder(ctx context.Context) int {
	configs, err := m.repo.List(ctx)
	if err != nil || len(configs) == 0 {
		return 0
	}
	lowest := configs[0].SortOrder
	for _, c := range configs {
		lowest = min(lowest, c.SortOrder)
	}
	return lowest - 1
}

// reorderFromDB re-applies the persisted order to the live registry (after a
// registration that may have appended a provider out of order).
func (m *ProviderManager) reorderFromDB(ctx context.Context) {
	configs, err := m.repo.List(ctx)
	if err != nil {
		return
	}
	order := make([]string, 0, len(configs))
	for _, c := range configs {
		order = append(order, c.Name)
	}
	m.registry.Reorder(order)
}
