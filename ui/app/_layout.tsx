import '../global.css';

import { useEffect, useLayoutEffect } from 'react';
import { Platform, useWindowDimensions, View } from 'react-native';
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
import { useDownloads } from '../src/offline/store';
import { useOfflineCatalog } from '../src/offline/catalog';
import { TrackMenu } from '../src/components/trackMenu';
import { ToastHost } from '../src/components/Toast';
import { PlayerBar } from '../src/components/PlayerBar';
import { TopBar } from '../src/components/TopBar';
import { SearchOverlay } from '../src/components/SearchOverlay';
import { AccentScope } from '../src/components/AccentScope';
import { LaunchLoading } from '../src/components/LaunchLoading';
import { LibrarySidebar } from '../src/components/LibrarySidebar';
import { AdminSidebar } from '../src/components/AdminSidebar';
import { MobileDrawer } from '../src/components/MobileDrawer';
import { useUI } from '../src/stores/ui';
import { useLocale, useT } from '../src/i18n/store';
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
  const t = useT();
  const locale = useLocale((s) => s.preference);
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
  const hydrateDownloads = useDownloads((s) => s.hydrate);
  const hydrateOfflineCatalog = useOfflineCatalog((s) => s.hydrate);
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
    void hydrateDownloads();
    void hydrateOfflineCatalog();
    void detectSelf();
  }, [hydrateTheme, restore, hydratePlayer, initPlayer, loadRecents, hydrateUI, hydrateLocale, hydrateDownloads, hydrateOfflineCatalog, detectSelf]);

  // Web only: expo-router disables automatic document titles, so set the
  // browser tab title from the current route.
  // The stack hides the previous screen with aria-hidden while the pressable
  // that triggered the navigation still holds focus, which browsers flag.
  // Drop that focus in the same commit, before the accessibility tree updates.
  useLayoutEffect(() => {
    if (Platform.OS !== 'web') return;
    const el = document.activeElement;
    if (el instanceof HTMLElement && el.closest('[aria-hidden="true"]')) el.blur();
  }, [pathname]);

  useEffect(() => {
    if (Platform.OS === 'web') document.title = documentTitle(pathname);
  }, [pathname, locale]);

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
  // Inside the admin section the left rail becomes a page-nav menu instead of
  // the playlist library (immich-style).
  const inAdmin = pathname === '/admin' || pathname.startsWith('/admin/');

  // Hold a themed splash until the persisted theme is applied, so the UI never
  // flashes unthemed (light) content on reload.
  if (!themeHydrated) {
    return (
      <GestureHandlerRootView style={{ flex: 1 }}>
        <LaunchLoading />
      </GestureHandlerRootView>
    );
  }

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <QueryClientProvider client={queryClient}>
          <AccentScope>
          <StatusBar style={colorScheme === 'dark' ? 'light' : 'dark'} />
          {/* Top bar, navigator, then the player bar docked at the bottom — on every screen. */}
          <View style={{ flex: 1 }}>
            <TopBar wide={wide} />
            {/* Desktop: sidebar persists beside the content. Mobile: full width. */}
            <View style={{ flex: 1, flexDirection: 'row' }}>
              {showSidebar ? inAdmin ? <AdminSidebar /> : <LibrarySidebar /> : null}
              <View style={{ flex: 1 }}>
                <Stack
                  screenOptions={{
                    headerStyle: { backgroundColor: colors.background },
                    headerTitleStyle: { color: colors.foreground },
                    headerTintColor: colors.primary,
                    contentStyle: { backgroundColor: colors.background },
                    // Otherwise the back button falls back to the previous
                    // screen's route name — e.g. the literal "(tabs)" group
                    // segment when coming from a tab screen.
                    headerBackButtonDisplayMode: 'minimal',
                  }}
                >
                  <Stack.Screen name="index" options={{ headerShown: false }} />
                  <Stack.Screen name="login" options={{ headerShown: false }} />
                  <Stack.Screen name="setup" options={{ headerShown: false }} />
                  <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
                  {/* On desktop the top bar provides Back, so detail screens hide
                      their own stack header; on mobile they keep it. */}
                  <Stack.Screen name="album/[id]" options={{ title: t('media.album.label'), headerShown: !wide }} />
                  <Stack.Screen name="artist/[id]" options={{ title: t('media.artist.label'), headerShown: !wide }} />
                  <Stack.Screen name="profile/[username]" options={{ title: t('settings.profile'), headerShown: !wide }} />
                  <Stack.Screen name="genre/[id]" options={{ title: t('navigation.genre'), headerShown: !wide }} />
                  <Stack.Screen name="playlist/[id]" options={{ title: t('components.sidebar.playlist'), headerShown: !wide }} />
                  <Stack.Screen name="liked" options={{ title: t('components.sidebar.likedSongs'), headerShown: !wide }} />
                  <Stack.Screen name="local" options={{ title: t('media.local.title'), headerShown: !wide }} />
                  <Stack.Screen name="jam/[id]" options={{ title: t('navigation.jam'), headerShown: !wide }} />
                  <Stack.Screen
                    name="player"
                    options={{ presentation: 'modal', headerShown: false }}
                  />
                  <Stack.Screen name="queue" options={{ presentation: 'modal', title: t('media.player.queue') }} />
                  <Stack.Screen name="cast-target" options={{ presentation: 'modal', title: t('media.player.castTitle') }} />
                  <Stack.Screen name="ui-kit" options={{ title: t('navigation.uiKit'), headerShown: !wide }} />
                  <Stack.Screen name="devices" options={{ title: t('tools.devices.title'), headerShown: !wide }} />
                  <Stack.Screen name="api-tokens" options={{ title: t('tools.tokens.title'), headerShown: !wide }} />
                  <Stack.Screen name="import" options={{ title: t('navigation.import'), headerShown: false }} />
                  <Stack.Screen name="import/[id]" options={{ title: t('navigation.import'), headerShown: false }} />
                  <Stack.Screen name="discover" options={{ title: t('components.sidebar.publicPlaylists'), headerShown: !wide }} />
                </Stack>
              </View>
            </View>
            <PlayerBar />
          </View>
          <TrackMenu />
          <SearchOverlay />
          {!wide ? <MobileDrawer /> : null}
          <ToastHost />
          </AccentScope>
        </QueryClientProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
