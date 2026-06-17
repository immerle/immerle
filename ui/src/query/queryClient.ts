import { QueryClient } from '@tanstack/react-query';

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
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});
