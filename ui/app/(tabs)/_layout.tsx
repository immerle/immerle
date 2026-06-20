import { useWindowDimensions, View } from 'react-native';
import { Tabs } from 'expo-router';
import { SafeAreaInsetsContext, useSafeAreaInsets } from 'react-native-safe-area-context';
import { Ionicon } from '../../src/components/Ionicon';
import { MobileHeader } from '../../src/components/MobileHeader';
import { MobileTabBar } from '../../src/components/MobileTabBar';
import { PlayerBar } from '../../src/components/PlayerBar';
import { useAuth } from '../../src/auth/store';
import { useSearchUI } from '../../src/search/store';
import { useColors } from '../../src/theme/colors';
import { WIDE_BREAKPOINT } from '../../src/theme/layout';
import { useT } from '../../src/i18n/store';

/**
 * Adaptive navigator. On wide screens (web/tablet) the nav becomes a Spotify-
 * style **left sidebar** (`tabBarPosition: 'left'`, material variant); on narrow
 * screens it's the usual bottom tab bar. The now-playing bar is docked app-wide
 * from the root layout (see `PlayerBar`), so the bottom strip is reserved for
 * the player. Admin/Social tabs appear only when available.
 */
export default function TabsLayout() {
  const t = useT();
  const colors = useColors();
  const { width } = useWindowDimensions();
  const wide = width >= WIDE_BREAKPOINT;
  const insets = useSafeAreaInsets();
  const hasSocial = useAuth((s) => s.client?.has('social') ?? false);
  const openSearch = useSearchUI((s) => s.openSearch);

  const tabs = (
    <Tabs
      // On mobile, dock the now-playing bar just above the tab bar; on desktop
      // there's no bottom bar (nav lives in the sidebar), so render nothing.
      tabBar={(props) =>
        wide ? null : (
          <View>
            <PlayerBar embedded />
            <MobileTabBar {...props} />
          </View>
        )
      }
      screenOptions={{
        // Screens render their own titles; no nav header (Spotify-style).
        headerShown: false,
        tabBarActiveTintColor: colors.foreground,
        tabBarInactiveTintColor: colors.muted,
        // On desktop, navigation lives in the top bar + library sidebar, so the
        // tab bar is hidden; on mobile it's the usual bottom bar.
        tabBarStyle: wide
          ? { display: 'none' }
          : {
              backgroundColor: colors.background,
              borderTopColor: colors.border,
              borderTopWidth: 0,
              elevation: 0,
            },
        tabBarLabelStyle: { fontSize: 11, fontWeight: '600' },
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: t('home.tabs.home'),
          // On desktop, Home lives in the top bar, not the sidebar.
          href: wide ? null : undefined,
          tabBarIcon: ({ color, size }) => <Ionicon name="home" size={size} color={color} />,
        }}
      />
      <Tabs.Screen
        name="search"
        options={{
          title: t('home.tabs.search'),
          // On desktop, search lives in the top bar.
          href: wide ? null : undefined,
          tabBarIcon: ({ color, size }) => <Ionicon name="search" size={size} color={color} />,
        }}
        listeners={{
          // The "Recherche" tab opens the full-screen search overlay instead of
          // navigating to a page — there is no dedicated search screen anymore.
          tabPress: (e) => {
            e.preventDefault();
            openSearch();
          },
        }}
      />
      <Tabs.Screen
        name="playlists"
        options={{
          title: t('home.tabs.playlists'),
          tabBarIcon: ({ color, size }) => (
            <Ionicon name="list" size={size} color={color} />
          ),
        }}
      />
      <Tabs.Screen
        name="social"
        options={{
          title: t('home.tabs.social'),
          href: hasSocial ? undefined : null,
          tabBarIcon: ({ color, size }) => <Ionicon name="people" size={size} color={color} />,
        }}
      />
      <Tabs.Screen
        name="admin"
        options={{
          title: t('home.tabs.admin'),
          // Reached from the avatar menu (header on mobile, top bar on desktop),
          // so it's kept out of the bottom bar to avoid crowding it.
          href: null,
          tabBarIcon: ({ color, size }) => (
            <Ionicon name="shield-checkmark" size={size} color={color} />
          ),
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{
          title: t('home.tabs.settings'),
          // Lives in the avatar menu, not the bottom bar.
          href: null,
          tabBarIcon: ({ color, size }) => (
            <Ionicon name="settings" size={size} color={color} />
          ),
        }}
      />
    </Tabs>
  );

  if (wide) return tabs;

  // Mobile: a global header above the tabs. It owns the top safe-area, so the
  // tab subtree's top inset is zeroed to avoid a double gap under the notch.
  return (
    <View className="flex-1 bg-background">
      <MobileHeader />
      <SafeAreaInsetsContext.Provider value={{ top: 0, left: insets.left, right: insets.right, bottom: insets.bottom }}>
        {tabs}
      </SafeAreaInsetsContext.Provider>
    </View>
  );
}
