import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { useAuth } from '../auth/store';
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
 * Live Subsonic search. The query string is expected to be already debounced
 * by the caller via {@link useDebounced}. Results are not cached and the prior
 * result is not kept on screen, so retyping shows the loading state every time.
 */
export function useSearch(query: string) {
  const client = useAuth((s) => s.client);
  const trimmed = query.trim();
  return useQuery({
    queryKey: qk.search(trimmed),
    enabled: !!client && trimmed.length > 0,
    staleTime: 0,
    gcTime: 0,
    queryFn: ({ signal }) => client!.search(trimmed),
  });
}
