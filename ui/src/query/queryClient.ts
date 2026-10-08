import { QueryClient } from '@tanstack/react-query';
import { ImmerleApiError } from '../api/immerle/types';
import { SubsonicApiError } from '../api/subsonic/types';

/** Retry once only when the server actually answered. A network failure or a
 * timeout means it's unreachable: retrying would just double the wait before
 * the offline fallback shows up. */
export function shouldRetry(failureCount: number, error: unknown): boolean {
  return failureCount < 1 && (error instanceof ImmerleApiError || error instanceof SubsonicApiError);
}

/**
 * Shared TanStack Query client. Music metadata changes rarely, so we keep a
 * generous stale time and lean on cache for snappy navigation; admin data
 * (scan progress, jobs) overrides these per-query with short intervals.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      gcTime: 30 * 60 * 1000,
      retry: shouldRetry,
      refetchOnWindowFocus: false,
    },
  },
});
