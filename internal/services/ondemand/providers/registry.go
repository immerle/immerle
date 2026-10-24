package providers

import (
	"fmt"
	"sync"
)

// Factory builds a Provider from its parsed Config (credentials and options).
// Providers self-register a Factory via RegisterFactory in init(), so
// third-party plugins (e.g. a Deezer/Qobuz downloader compiled into the binary)
// can plug in without changes to the core: they register a factory and read
// their credentials — such as a Deezer ARL — from the config Params.
type Factory func(cfg Config) (Provider, error)

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

// Build instantiates the named provider from its parsed config.
func Build(name string, cfg Config) (Provider, error) {
	factoriesMu.RLock()
	f, ok := factories[name]
	factoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no provider factory registered for %q", name)
	}
	return f(cfg)
}
