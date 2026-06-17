import { useWindowDimensions } from 'react-native';
import { Tabs } from 'expo-router';
import { Ionicon } from '../../src/components/Ionicon';
import { useAuth } from '../../src/auth/store';
import { useSearchUI } from '../../src/search/store';
import { useColors } from '../../src/theme/colors';
import { WIDE_BREAKPOINT } from '../../src/theme/layout';

/**
 * Adaptive navigator. On wide screens (web/tablet) the nav becomes a Spotify-
 * style **left sidebar** (`tabBarPosition: 'left'`, material variant); on narrow
 * screens it's the usual bottom tab bar. The now-playing bar is docked app-wide
 * from the root layout (see `PlayerBar`), so the bottom strip is reserved for
 * the player. Admin/Social tabs appear only when available.
 */
export default function TabsLayout() {
  const colors = useColors();
  const { width } = useWindowDimensions();
  const wide = width >= WIDE_BREAKPOINT;
  const isAdmin = useAuth((s) => s.client?.isAdmin ?? false);
  const hasSocial = useAuth((s) => s.client?.has('social') ?? false);
  const openSearch = useSearchUI((s) => s.openSearch);

  return (
    <Tabs
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
          title: 'Accueil',
          // On desktop, Home lives in the top bar, not the sidebar.
          href: wide ? null : undefined,
          tabBarIcon: ({ color, size }) => <Ionicon name="home" size={size} color={color} />,
        }}
      />
      <Tabs.Screen
        name="search"
        options={{
          title: 'Recherche',
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
          title: 'Playlists',
          tabBarIcon: ({ color, size }) => (
            <Ionicon name="list" size={size} color={color} />
          ),
        }}
      />
      <Tabs.Screen
        name="social"
        options={{
          title: 'Social',
          href: hasSocial ? undefined : null,
          tabBarIcon: ({ color, size }) => <Ionicon name="people" size={size} color={color} />,
        }}
      />
      <Tabs.Screen
        name="admin"
        options={{
          title: 'Admin',
          // Admins reach this from the avatar menu on desktop; bottom tab on mobile.
          href: wide || !isAdmin ? null : undefined,
          tabBarIcon: ({ color, size }) => (
            <Ionicon name="shield-checkmark" size={size} color={color} />
          ),
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{
          title: 'Réglages',
          // On desktop, settings is in the avatar menu.
          href: wide ? null : undefined,
          tabBarIcon: ({ color, size }) => (
            <Ionicon name="settings" size={size} color={color} />
          ),
        }}
      />
    </Tabs>
  );
}
