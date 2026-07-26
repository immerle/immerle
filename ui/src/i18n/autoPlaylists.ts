/** Maps a playlist's `autoPlaylistKind` — an internal/autoplaylists.
 * AutoPlaylistKinds value, or a kworb chart's own sourceExternalID
 * (internal/charts.SourceInstanceID, e.g. "fr_weekly") — to its i18n key
 * under media.playlist.autoKind. Kept as a lookup table, not a fixed union,
 * so an unrecognized/future kind falls back to the raw (French-only) stored
 * name instead of throwing. */
const AUTO_PLAYLIST_KIND_KEYS: Record<string, string> = {
  'top-month-mix': 'media.playlist.autoKind.topMonthMix',
  'on-repeat-mix': 'media.playlist.autoKind.onRepeatMix',
  'forgotten-mix': 'media.playlist.autoKind.forgottenMix',
  'random-mix': 'media.playlist.autoKind.randomMix',
  'recommended-mix': 'media.playlist.autoKind.recommendedMix',
  'weekly-trending-mix': 'media.playlist.autoKind.weeklyTrendingMix',
  'listenbrainz-daily-jams': 'media.playlist.autoKind.listenBrainzDailyJams',
  'listenbrainz-weekly-jams': 'media.playlist.autoKind.listenBrainzWeeklyJams',
  'listenbrainz-weekly-exploration': 'media.playlist.autoKind.listenBrainzWeeklyExploration',
  'lastfm-similar-mix': 'media.playlist.autoKind.lastfmSimilarMix',
  'global_weekly': 'media.playlist.autoKind.chartGlobal',
  'fr_weekly': 'media.playlist.autoKind.chartFr',
  'us_weekly': 'media.playlist.autoKind.chartUs',
  'gb_weekly': 'media.playlist.autoKind.chartGb',
  'de_weekly': 'media.playlist.autoKind.chartDe',
  'es_weekly': 'media.playlist.autoKind.chartEs',
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

/** Maps an auto-generated playlist's kind to the external recommendation
 * engine that built it — shown as a subtitle so a "Discover" or "Daily Jams"
 * playlist doesn't read as if immerle itself picked the tracks. undefined for
 * every other kind (charts, genre/decade, personal listening lists), which
 * are all sourced from the local library/scrobbles, not a third party. */
const AUTO_PLAYLIST_SOURCE_KEYS: Record<string, string> = {
  'recommended-mix': 'media.playlist.source.reccobeats',
  'listenbrainz-daily-jams': 'media.playlist.source.listenbrainz',
  'listenbrainz-weekly-jams': 'media.playlist.source.listenbrainz',
  'listenbrainz-weekly-exploration': 'media.playlist.source.listenbrainz',
  'lastfm-similar-mix': 'media.playlist.source.lastfm',
};

export function autoPlaylistSource(t: (scope: string) => string, kind: string | undefined | null): string | undefined {
  const key = kind ? AUTO_PLAYLIST_SOURCE_KEYS[kind] : undefined;
  return key ? t(key) : undefined;
}
