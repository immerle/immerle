import '../global.css';

import { useEffect } from 'react';
import { ActivityIndicator, Platform, useWindowDimensions, View } from 'react-native';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { StatusBar } from 'expo-status-bar';
import { Stack, usePathname } from 'expo-router';
import { QueryClientProvider } from '@tanstack/react-query';
import { useColorScheme } from 'nativewind';
import { queryClient } from '../src/query/queryClient';
import { useAuth } from '../src/auth/store';
import { useTheme } from '../src/theme/store';
import { usePlayer } from '../src/audio/store';
import { useSearchUI } from '../src/search/store';
import { TrackMenu } from '../src/components/trackMenu';
import { PlayerBar } from '../src/components/PlayerBar';
import { TopBar } from '../src/components/TopBar';
import { SearchOverlay } from '../src/components/SearchOverlay';
import { AccentScope } from '../src/components/AccentScope';
import { LibrarySidebar } from '../src/components/LibrarySidebar';
import { useUI } from '../src/stores/ui';
import { useLocale } from '../src/i18n/store';
import { useSelfServer } from '../src/api/selfServer';
import { palette } from '../src/theme/colors';
import { WIDE_BREAKPOINT } from '../src/theme/layout';
import { documentTitle } from '../src/utils/documentTitle';
import { installRouterKeyStripper } from '../src/utils/routerKey';

/**
 * Root layout: wires global providers (gesture handler, safe area, query
 * client), boots one-time state (theme, persisted session, audio engine), and
 * declares the navigation stack. The auth gate itself lives in `index.tsx`,
 * which redirects based on the restored session.
 */
export default function RootLayout() {
  const { colorScheme } = useColorScheme();
  const { width } = useWindowDimensions();
  const pathname = usePathname();
  const wide = width >= WIDE_BREAKPOINT;
  const restore = useAuth((s) => s.restore);
  const hydrateTheme = useTheme((s) => s.hydrate);
  const syncTheme = useTheme((s) => s.syncFromServer);
  const initPlayer = usePlayer((s) => s.init);
  const hydratePlayer = usePlayer((s) => s.hydrateSettings);
  const loadRecents = useSearchUI((s) => s.loadRecents);
  const hydrateUI = useUI((s) => s.hydrate);
  const hydrateLocale = useLocale((s) => s.hydrate);
  const detectSelf = useSelfServer((s) => s.detect);
  const authStatus = useAuth((s) => s.status);
  const themeHydrated = useTheme((s) => s.hydrated);

  useEffect(() => {
    installRouterKeyStripper();
    void hydrateTheme();
    void restore();
    void hydratePlayer();
    void initPlayer();
    void loadRecents();
    void hydrateUI();
    void hydrateLocale();
    void detectSelf();
  }, [hydrateTheme, restore, hydratePlayer, initPlayer, loadRecents, hydrateUI, hydrateLocale, detectSelf]);

  // Web only: expo-router disables automatic document titles, so set the
  // browser tab title from the current route.
  useEffect(() => {
    if (Platform.OS === 'web') document.title = documentTitle(pathname);
  }, [pathname]);

  // Once the session is authenticated, the server is the source of truth for
  // the accent — pull it so the choice follows the user across devices.
  useEffect(() => {
    if (authStatus === 'authenticated') void syncTheme();
  }, [authStatus, syncTheme]);

  const colors = colorScheme === 'dark' ? palette.dark : palette.light;
  // The desktop library sidebar shows once authenticated, beside the content —
  // except on the immersive full-screen surfaces (player / queue).
  const immersive = pathname === '/player' || pathname === '/queue';
  const showSidebar = wide && authStatus === 'authenticated' && !immersive;

  // Hold a themed splash until the persisted theme is applied, so the UI never
  // flashes unthemed (light) content on reload.
  if (!themeHydrated) {
    return (
      <GestureHandlerRootView style={{ flex: 1 }}>
        <View style={{ flex: 1, backgroundColor: palette.dark.background, alignItems: 'center', justifyContent: 'center' }}>
          <ActivityIndicator color={palette.dark.primary} />
        </View>
      </GestureHandlerRootView>
    );
  }

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <QueryClientProvider client={queryClient}>
          <AccentScope>
          <StatusBar style={colorScheme === 'dark' ? 'light' : 'dark'} />
          {/* Column: a global top bar (desktop), the navigator in the middle,
              and the player bar docked at the very bottom — foreground on every
              screen. */}
          <View style={{ flex: 1 }}>
            <TopBar wide={wide} />
            {/* On desktop, the library sidebar sits beside the navigator content
                and persists across screens; on mobile the navigator is full width. */}
            <View style={{ flex: 1, flexDirection: 'row' }}>
              {showSidebar ? <LibrarySidebar /> : null}
              <View style={{ flex: 1 }}>
                <Stack
                  screenOptions={{
                    headerStyle: { backgroundColor: colors.background },
                    headerTitleStyle: { color: colors.foreground },
                    headerTintColor: colors.primary,
                    contentStyle: { backgroundColor: colors.background },
                  }}
                >
                  <Stack.Screen name="index" options={{ headerShown: false }} />
                  <Stack.Screen name="login" options={{ headerShown: false }} />
                  <Stack.Screen name="setup" options={{ headerShown: false }} />
                  <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
                  {/* On desktop the top bar provides Back, so detail screens hide
                      their own stack header; on mobile they keep it. */}
                  <Stack.Screen name="album/[id]" options={{ title: 'Album', headerShown: !wide }} />
                  <Stack.Screen name="artist/[id]" options={{ title: 'Artiste', headerShown: !wide }} />
                  <Stack.Screen name="profile/[username]" options={{ title: 'Profil', headerShown: !wide }} />
                  <Stack.Screen name="genre/[id]" options={{ title: 'Genre', headerShown: !wide }} />
                  <Stack.Screen name="playlist/[id]" options={{ title: 'Playlist', headerShown: !wide }} />
                  <Stack.Screen name="liked" options={{ title: 'Titres likés', headerShown: !wide }} />
                  <Stack.Screen name="local" options={{ title: 'Musiques locales', headerShown: !wide }} />
                  <Stack.Screen name="jam/[id]" options={{ title: 'Jam', headerShown: !wide }} />
                  <Stack.Screen
                    name="player"
                    options={{ presentation: 'modal', headerShown: false }}
                  />
                  <Stack.Screen name="queue" options={{ presentation: 'modal', title: 'File de lecture' }} />
                  <Stack.Screen name="ui-kit" options={{ title: 'UI Kit', headerShown: !wide }} />
                  <Stack.Screen name="devices" options={{ title: 'Appareils connectés', headerShown: !wide }} />
                  <Stack.Screen name="api-tokens" options={{ title: 'API', headerShown: !wide }} />
                  <Stack.Screen name="import" options={{ title: 'Importer', headerShown: false }} />
                  <Stack.Screen name="import/[id]" options={{ title: 'Import', headerShown: false }} />
                  <Stack.Screen name="discover" options={{ title: 'Playlists publiques', headerShown: !wide }} />
                </Stack>
              </View>
            </View>
            <PlayerBar />
          </View>
          <TrackMenu />
          <SearchOverlay />
          </AccentScope>
        </QueryClientProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
