import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { useAuth } from '../auth/store';
import { SearchTypeFilter } from '../search/store';
import { qk } from './keys';

/** Debounce any fast-changing value (used for live search input). */
export function useDebounced<T>(value: T, delayMs = 250): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(t);
  }, [value, delayMs]);
  return debounced;
}

/**
 * Live search, scoped server-side to `typeFilter`. Caller must debounce the
 * query (see {@link useDebounced}). Not cached (staleTime/gcTime 0), so
 * retyping or switching the filter always shows the loading state.
 */
export function useSearch(query: string, typeFilter: SearchTypeFilter) {
  const client = useAuth((s) => s.client);
  const trimmed = query.trim();
  return useQuery({
    queryKey: qk.search(trimmed, typeFilter),
    enabled: !!client && trimmed.length > 0,
    staleTime: 0,
    gcTime: 0,
    queryFn: ({ signal }) => client!.search(trimmed, typeFilter === 'all' ? undefined : typeFilter, signal),
  });
}
