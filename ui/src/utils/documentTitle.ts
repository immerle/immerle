const APP_NAME = 'Immerle';

// Route-segment → tab title. expo-router disables automatic web document titles
// (NavigationContainer documentTitle.enabled = false), so we set them ourselves.
// Labels mirror the stack header titles in app/_layout.tsx.
const TITLES: Record<string, string> = {
  '': 'Accueil',
  login: 'Connexion',
  setup: 'Configuration',
  playlists: 'Playlists',
  social: 'Social',
  settings: 'Réglages',
  admin: 'Admin',
  album: 'Album',
  artist: 'Artiste',
  profile: 'Profil',
  genre: 'Genre',
  playlist: 'Playlist',
  liked: 'Titres likés',
  jam: 'Jam',
  player: 'Lecture en cours',
  queue: 'File de lecture',
  'ui-kit': 'UI Kit',
  devices: 'Appareils connectés',
  'api-tokens': 'API',
  import: 'Importer',
  discover: 'Playlists publiques',
};

const ADMIN_SUB: Record<string, string> = {
  jobs: 'Jobs',
  providers: 'Providers',
  scan: 'Scan',
  settings: 'Réglages',
  users: 'Utilisateurs',
};

/** Browser tab title for a given route pathname, e.g. "Album · Immerle". */
export function documentTitle(pathname: string): string {
  const seg = pathname.split('/').filter(Boolean);
  const root = seg[0] ?? '';
  const label =
    root === 'admin' && seg[1] ? (ADMIN_SUB[seg[1]] ?? 'Admin') : TITLES[root];
  return label ? `${label} · ${APP_NAME}` : APP_NAME;
}
