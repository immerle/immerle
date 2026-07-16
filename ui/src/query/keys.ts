import type { QueryClient } from '@tanstack/react-query';

/** Centralized query-key factory so invalidation stays consistent. */
export const qk = {
  ping: ['ping'] as const,
  artist: (id: string) => ['artist', id] as const,
  album: (id: string) => ['album', id] as const,
  albumList: (type: string, genre?: string) => ['albumList', type, genre ?? null] as const,
  songsByGenre: (genre: string) => ['songsByGenre', genre] as const,
  lyrics: (id: string) => ['lyrics', id] as const,
  search: (query: string, type: string) => ['search', query, type] as const,
  starred: ['starred'] as const,
  local: ['local'] as const,
  customPlaylists: ['customPlaylists'] as const,

  smartPlaylists: ['smartPlaylists'] as const,
  smartPlaylist: (id: string) => ['smartPlaylists', id] as const,
  smartPlaylistsAdmin: ['admin', 'smartPlaylists'] as const,
  radio: ['radio'] as const,
  radioAdmin: ['admin', 'radio'] as const,
  wrapped: (year: number) => ['wrapped', year] as const,
  wrappedAdmin: ['admin', 'wrapped'] as const,
  offlineAdmin: ['admin', 'offline'] as const,
  hallOfFame: ['hallOfFame'] as const,
  hallOfFameAdmin: ['admin', 'hallOfFame'] as const,

  playlists: ['playlists'] as const,
  playlist: (id: string) => ['playlist', id] as const,
  publicPlaylists: ['playlists', 'public'] as const,

  // Admin
  libraryStats: ['admin', 'libraryStats'] as const,
  adminTracks: (query: string) => ['admin', 'tracks', query] as const,
  scanProgress: ['admin', 'scanProgress'] as const,
  providers: ['admin', 'providers'] as const,
  providerLogs: (name: string) => ['admin', 'providers', name, 'logs'] as const,
  jobs: ['admin', 'jobs'] as const,
  federation: ['admin', 'federation'] as const,
  federationSubscriptions: ['admin', 'federation', 'subscriptions'] as const,
  federationSearch: (q: string) => ['admin', 'federation', 'search', q] as const,
  settings: ['admin', 'settings'] as const,
  cleanup: ['admin', 'cleanup'] as const,
} as const;

/**
 * Invalidates every cache that can independently render a track's identity
 * (album/artist detail, browse lists, search, favorites, playlists, Wrapped) —
 * a Song is embedded separately in each, so one mutation (track edit/delete,
 * favorite toggle, scan) must refresh them all, not just the triggering screen.
 */
export function invalidateCatalog(qc: QueryClient): void {
  qc.invalidateQueries({ queryKey: ['artist'] });
  qc.invalidateQueries({ queryKey: ['album'] });
  qc.invalidateQueries({ queryKey: ['albumList'] });
  qc.invalidateQueries({ queryKey: ['songsByGenre'] });
  qc.invalidateQueries({ queryKey: ['search'] });
  qc.invalidateQueries({ queryKey: qk.starred });
  qc.invalidateQueries({ queryKey: ['playlist'] });
  qc.invalidateQueries({ queryKey: ['wrapped'] });
}
