/** Centralized query-key factory so invalidation stays consistent. */
export const qk = {
  ping: ['ping'] as const,
  artists: ['artists'] as const,
  artist: (id: string) => ['artist', id] as const,
  album: (id: string) => ['album', id] as const,
  albumList: (type: string, genre?: string) => ['albumList', type, genre ?? null] as const,
  genres: ['genres'] as const,
  songsByGenre: (genre: string) => ['songsByGenre', genre] as const,
  search: (query: string) => ['search', query] as const,
  starred: ['starred'] as const,
  local: ['local'] as const,

  wrapped: (year: number) => ['wrapped', year] as const,
  wrappedAdmin: ['admin', 'wrapped'] as const,

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
  transcodeProfiles: ['admin', 'transcodeProfiles'] as const,
  serverSettings: ['admin', 'serverSettings'] as const,
  settings: ['admin', 'settings'] as const,
  cleanup: ['admin', 'cleanup'] as const,
} as const;
