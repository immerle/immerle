/** Maps a playlist's `autoPlaylistKind` (see internal/autoplaylists.AutoPlaylistKinds
 * on the backend) to its i18n key under media.playlist.autoKind. Kept as a
 * lookup table, not a fixed union, so an unrecognized/future kind falls back
 * to the raw (French-only) stored name instead of throwing. */
const AUTO_PLAYLIST_KIND_KEYS: Record<string, string> = {
  'top-month-mix': 'media.playlist.autoKind.topMonthMix',
  'on-repeat-mix': 'media.playlist.autoKind.onRepeatMix',
  'forgotten-mix': 'media.playlist.autoKind.forgottenMix',
  'random-mix': 'media.playlist.autoKind.randomMix',
  'recommended-mix': 'media.playlist.autoKind.recommendedMix',
  'weekly-trending-mix': 'media.playlist.autoKind.weeklyTrendingMix',
};

/** Returns a locale-appropriate name for a playlist: translated when it's one
 * of the server-generated kinds, else the playlist's own stored name
 * unchanged (a user-created, genre/decade or hub-imported playlist's name
 * isn't a translation key). */
export function autoPlaylistName(
  t: (scope: string, params?: Record<string, unknown>) => string,
  kind: string | undefined | null,
  fallbackName: string
): string {
  const key = kind ? AUTO_PLAYLIST_KIND_KEYS[kind] : undefined;
  return key ? t(key) : fallbackName;
}
