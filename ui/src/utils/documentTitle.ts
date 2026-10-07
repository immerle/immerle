import { useEffect } from 'react';
import { Platform } from 'react-native';
import { t } from '../i18n';

const APP_NAME = 'Immerle';

// Route-segment → tab title. expo-router disables automatic web document titles
// (NavigationContainer documentTitle.enabled = false), so we set them ourselves.
// Labels mirror the stack header titles in app/_layout.tsx.
const TITLES: Record<string, string> = {
  '': 'navigation.home',
  login: 'navigation.login',
  setup: 'navigation.setup',
  playlists: 'navigation.playlists',
  social: 'navigation.social',
  settings: 'settings.title',
  admin: 'settings.admin',
  album: 'media.album.label',
  artist: 'media.artist.label',
  profile: 'settings.profile',
  genre: 'navigation.genre',
  playlist: 'components.sidebar.playlist',
  liked: 'components.sidebar.likedSongs',
  local: 'media.local.title',
  jam: 'navigation.jam',
  player: 'media.player.nowPlaying',
  queue: 'media.player.queue',
  'cast-target': 'media.player.castTitle',
  'ui-kit': 'navigation.uiKit',
  devices: 'tools.devices.title',
  'api-tokens': 'tools.tokens.title',
  import: 'navigation.import',
  discover: 'components.sidebar.publicPlaylists',
  offline: 'offline.title',
  wrapped: 'wrapped.title',
  bandcamp: 'bandcamp.title',
  halloffame: 'media.hallOfFame.title',
  'smart-playlists': 'smart.title',
  'smart-playlist': 'smart.title',
  radios: 'radio.title',
};

const ADMIN_SUB: Record<string, string> = {
  jobs: 'navigation.jobs',
  providers: 'navigation.providers',
  scan: 'navigation.scan',
  settings: 'settings.title',
  users: 'navigation.users',
  tracks: 'navigation.tracks',
  radio: 'radio.manageTitle',
  federation: 'navigation.federation',
  logs: 'navigation.logs',
};

/** Browser tab title for a given route pathname, e.g. "Album · Immerle". */
export function documentTitle(pathname: string): string {
  const seg = pathname.split('/').filter(Boolean);
  const root = seg[0] ?? '';
  const label =
    root === 'admin' && seg[1] ? (ADMIN_SUB[seg[1]] ?? 'settings.admin') : TITLES[root];
  return label ? `${t(label)} · ${APP_NAME}` : APP_NAME;
}

/**
 * Web only: override the browser tab title with a loaded entity name, e.g.
 * "Daft Punk · Immerle". The root layout sets a generic per-route title on
 * navigation; detail screens call this to replace it once their data resolves.
 */
export function useWebTitle(name: string | null | undefined): void {
  useEffect(() => {
    if (Platform.OS === 'web' && name) document.title = `${name} · ${APP_NAME}`;
  }, [name]);
}
