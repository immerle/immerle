package core

import (
	"context"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/providers"
)

// searchProvider returns the single provider used for remote search: the first
// one by admin-controlled order. Search intentionally does NOT fan out to every
// provider (that multiplies network latency); it targets one provider only.
func (s *CatalogService) searchProvider() (providers.Provider, bool) {
	all := s.state.registry.All()
	if len(all) == 0 {
		return nil, false
	}
	return all[0], true
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
