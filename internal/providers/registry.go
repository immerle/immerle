package providers

import (
	"fmt"
	"sync"
)

// Factory builds a Provider from its per-provider settings (credentials and
// options). Providers self-register a Factory via RegisterFactory in init(), so
// third-party plugins (e.g. a Deezer/Qobuz downloader compiled into the binary)
// can plug in without changes to the core: they register a factory and read
// their credentials — such as a Deezer ARL — from the settings map.
type Factory func(settings map[string]string) (Provider, error)

var (
	factoriesMu sync.RWMutex
	factories   = map[string]Factory{}
)

// RegisterFactory registers a provider factory under name. Safe for use in init().
func RegisterFactory(name string, f Factory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[name] = f
}

// HasFactory reports whether a factory is registered for name.
func HasFactory(name string) bool {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	_, ok := factories[name]
	return ok
}

// Build instantiates the named provider from its settings.
func Build(name string, settings map[string]string) (Provider, error) {
	factoriesMu.RLock()
	f, ok := factories[name]
	factoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no provider factory registered for %q", name)
	}
	return f(settings)
}

// setting returns settings[key] or a fallback default.
func setting(settings map[string]string, key, def string) string {
	if v, ok := settings[key]; ok && v != "" {
		return v
	}
	return def
}
