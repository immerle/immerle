package core

import (
	"context"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/providers"
)

// searchProvider returns the single provider used for a one-provider lookup:
// the first one, by admin-controlled order, whose capability key is enabled.
// These lookups intentionally do NOT fan out to every provider (that
// multiplies network latency); they target one provider only.
func (s *CatalogService) searchProvider(ctx context.Context, key string) (providers.Provider, bool) {
	for _, prov := range s.state.registry.All() {
		if s.capable(ctx, prov, key) {
			return prov, true
		}
	}
	return nil, false
}

// capable reports whether prov's effective capabilities (declared, minus the
// ones the admin switched off) include key.
func (s *CatalogService) capable(ctx context.Context, prov providers.Provider, key string) bool {
	return s.cachedCapabilities(ctx, prov).Has(key)
}

// providerAs returns prov as T when it implements T and capability key is
// enabled for it: the single gate in front of every optional provider call.
func providerAs[T any](ctx context.Context, s *CatalogService, prov providers.Provider, key string) (T, bool) {
	t, ok := prov.(T)
	if !ok || !s.capable(ctx, prov, key) {
		var zero T
		return zero, false
	}
	return t, true
}

// searchCtx derives a bounded context for a remote search so a slow provider
// cannot stall the request.
func (s *CatalogService) searchCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.state.settings.SearchTimeout())
}

func (s *CatalogService) cacheGet(key string) (any, bool) {
	st := s.state
	st.searchMu.Lock()
	defer st.searchMu.Unlock()
	e, ok := st.searchCache[key]
	if !ok || time.Since(e.at) > st.searchTTL {
		return nil, false
	}
	return e.val, true
}

func (s *CatalogService) cachePut(key string, val any) {
	st := s.state
	st.searchMu.Lock()
	defer st.searchMu.Unlock()
	// Bound the cache; drop everything if it grows large (cheap, rarely hit).
	if len(st.searchCache) > 512 {
		st.searchCache = map[string]searchCacheEntry{}
	}
	st.searchCache[key] = searchCacheEntry{at: time.Now(), val: val}
}

// cachedTrackSearch runs prov.Search through a TTL cache, deduplicating
// concurrent identical lookups (e.g. the song and artist passes of one request)
// via singleflight so the provider is hit at most once per query.
func (s *CatalogService) cachedTrackSearch(ctx context.Context, prov providers.Provider, query string, limit int) ([]providers.Result, error) {
	if !s.capable(ctx, prov, providers.CapTrackSearch) {
		return nil, nil
	}
	key := "t|" + prov.Name() + "|" + strings.ToLower(query)
	if v, ok := s.cacheGet(key); ok {
		return v.([]providers.Result), nil
	}
	v, err, _ := s.state.searchSF.Do(key, func() (any, error) {
		res, err := prov.Search(ctx, query, limit)
		if err != nil {
			return nil, err
		}
		s.cachePut(key, res)
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]providers.Result), nil
}

// cachedCapabilities returns prov's effective capabilities: what it declares
// (cached; DefaultCapabilities when it can't say) minus the keys the admin
// switched off.
func (s *CatalogService) cachedCapabilities(ctx context.Context, prov providers.Provider) providers.Capabilities {
	return s.declaredCapabilities(ctx, prov).Without(s.state.registry.DisabledCapabilities(prov.Name()))
}

func (s *CatalogService) declaredCapabilities(ctx context.Context, prov providers.Provider) providers.Capabilities {
	cp, ok := prov.(providers.CapabilityProvider)
	if !ok {
		return providers.DefaultCapabilities()
	}
	key := "caps|" + prov.Name()
	if v, ok := s.cacheGet(key); ok {
		return v.(providers.Capabilities)
	}
	v, err, _ := s.state.searchSF.Do(key, func() (any, error) {
		caps, err := cp.Capabilities(ctx)
		if err != nil {
			return nil, err
		}
		s.cachePut(key, caps)
		return caps, nil
	})
	if err != nil {
		return providers.DefaultCapabilities() // fail open: keep searching a provider whose /capabilities flaps
	}
	return v.(providers.Capabilities)
}

// cachedArtistSearch runs an ArtistSearcher through the same cache machinery.
func (s *CatalogService) cachedArtistSearch(ctx context.Context, prov providers.ArtistSearcher, name, query string, limit int) ([]providers.ArtistResult, error) {
	key := "a|" + name + "|" + strings.ToLower(query)
	if v, ok := s.cacheGet(key); ok {
		return v.([]providers.ArtistResult), nil
	}
	v, err, _ := s.state.searchSF.Do(key, func() (any, error) {
		res, err := prov.SearchArtists(ctx, query, limit)
		if err != nil {
			return nil, err
		}
		s.cachePut(key, res)
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]providers.ArtistResult), nil
}
