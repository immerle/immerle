import { useState } from 'react';
import { Image, Modal, Pressable, Text, TextInput, View } from 'react-native';
import { router, usePathname } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Ionicon } from './Ionicon';
import { IconButton } from './ui';
import { NotificationsBell } from './NotificationsBell';
import { SearchTypeFilterButton } from './SearchTypeFilter';
import { useAuth } from '../auth/store';
import { useSearchUI } from '../search/store';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

// The top bar is the desktop chrome; it provides Back so detail screens drop
// their own header. Hidden only on pre-auth and the full-screen player/queue.
const HIDDEN_ROUTES = ['/login', '/setup', '/player', '/queue'];

/**
 * Global desktop header: a back/home cluster on the left, a search affordance in
 * the middle, and the current user's avatar on the right. The avatar opens a
 * menu with Settings, Admin (admins only) and logout — mirroring the Spotify web
 * top bar. Rendered app-wide from the root layout; self-hides on mobile, on
 * pre-auth screens, and on detail pages.
 */
export function TopBar({ wide }: { wide: boolean }) {
  const t = useT();
  const colors = useColors();
  const insets = useSafeAreaInsets();
  const pathname = usePathname();
  const client = useAuth((s) => s.client);
  const status = useAuth((s) => s.status);
  const displayNameState = useAuth((s) => s.displayName);
  const logout = useAuth((s) => s.logout);
  const query = useSearchUI((s) => s.query);
  const setQuery = useSearchUI((s) => s.setQuery);
  const openSearch = useSearchUI((s) => s.openSearch);
  const closeSearch = useSearchUI((s) => s.close);
  const [menu, setMenu] = useState(false);
  const [searchFocused, setSearchFocused] = useState(false);

  if (!wide || status !== 'authenticated' || HIDDEN_ROUTES.includes(pathname)) return null;

  const displayName = displayNameState ?? client?.username ?? '?';
  const initial = displayName.charAt(0).toUpperCase();
  const isAdmin = client?.isAdmin ?? false;
  const hasSocial = client?.has('social') ?? false;

  const go = (href: string) => {
    setMenu(false);
    router.navigate(href as never);
  };
  // Desktop chrome only (web/electron) — mirror the browser's forward button.
  const goForward = () => {
    if (typeof window !== 'undefined' && window.history) window.history.forward();
  };
  const onLogout = async () => {
    setMenu(false);
    await logout();
    router.replace('/login');
  };

  return (
    <View
      className="flex-row items-center gap-3 border-b border-border bg-background px-4"
      style={{ paddingTop: insets.top, height: 56 + insets.top }}
    >
      {/* Left — logo */}
      <Pressable onPress={() => go('/')} accessibilityRole="button" accessibilityLabel="Immerle">
        <Image
          source={require('../../assets/logo.png')}
          style={{ height: 32, width: 32 * (480 / 391) }}
          resizeMode="contain"
        />
      </Pressable>

      {/* Center — home + live search input, then back/forward */}
      <View className="flex-1 flex-row items-center justify-center gap-2">
        <CircleButton icon="home" onPress={() => go('/')} active={pathname === '/'} label={t('components.topbar.home')} />
        <View
          className={`max-w-[520px] flex-1 flex-row items-center gap-2 rounded-full border bg-surface-alt px-4 ${
            searchFocused ? 'border-primary' : 'border-transparent'
          }`}
        >
          <SearchTypeFilterButton />
          <Ionicon name="search" size={18} color={colors.muted} />
          <TextInput
            value={query}
            onChangeText={setQuery}
            onFocus={() => {
              openSearch();
              setSearchFocused(true);
            }}
            onBlur={() => setSearchFocused(false)}
            placeholder={t('components.topbar.searchPlaceholder')}
            placeholderTextColor={colors.muted}
            className="flex-1 py-2.5 text-sm text-foreground"
            autoCapitalize="none"
            autoCorrect={false}
            returnKeyType="search"
          />
          {query ? (
            <IconButton name="close-circle" size={18} color={colors.muted} onPress={closeSearch} accessibilityLabel={t('components.topbar.clear')} />
          ) : null}
        </View>
        <CircleButton icon="chevron-back" onPress={() => router.back()} label={t('components.topbar.back')} />
        <CircleButton icon="chevron-forward" onPress={goForward} label={t('components.topbar.forward')} />
      </View>

      {/* Right — jam + social + avatar */}
      <View className="flex-row items-center gap-3">
        <NotificationsBell />
        {hasSocial ? (
          <CircleButton icon="people" onPress={() => go('/social')} active={pathname === '/social'} label={t('components.topbar.social')} />
        ) : null}
        <Pressable
          onPress={() => setMenu(true)}
          accessibilityRole="button"
          accessibilityLabel={t('components.topbar.accountMenu')}
          className="h-9 w-9 items-center justify-center rounded-full bg-primary active:opacity-80"
        >
          <Text className="text-sm font-bold text-primary-foreground">{initial}</Text>
        </Pressable>
      </View>

      <Modal transparent visible={menu} animationType="fade" onRequestClose={() => setMenu(false)}>
        <Pressable className="flex-1" onPress={() => setMenu(false)}>
          <View
            className="absolute right-3 w-56 overflow-hidden rounded-xl border border-border bg-surface"
            style={{ top: insets.top + 52 }}
          >
            <View className="border-b border-border px-4 py-3">
              <Text numberOfLines={1} className="text-sm font-semibold text-foreground">
                {displayName}
              </Text>
              <Text numberOfLines={1} className="text-xs text-muted">
                {client?.displayName && client.displayName !== client.username ? `@${client.username} · ` : ''}
                {client?.serverUrl}
              </Text>
            </View>
            {hasSocial ? (
              <MenuItem icon="person-circle-outline" label={t('components.topbar.myProfile')} onPress={() => go('/profile/me')} />
            ) : null}
            <MenuItem icon="settings-outline" label={t('components.topbar.settings')} onPress={() => go('/settings')} />
            {isAdmin ? (
              <MenuItem icon="shield-checkmark-outline" label={t('components.topbar.administration')} onPress={() => go('/admin')} />
            ) : null}
            <MenuItem icon="log-out-outline" label={t('components.topbar.logout')} tone="danger" onPress={onLogout} />
          </View>
        </Pressable>
      </Modal>
    </View>
  );
}

function CircleButton({
  icon,
  onPress,
  active,
  label,
}: {
  icon: string;
  onPress: () => void;
  active?: boolean;
  label: string;
}) {
  const colors = useColors();
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      accessibilityLabel={label}
      className="h-9 w-9 items-center justify-center rounded-full bg-surface-alt active:opacity-70"
    >
      <Ionicon name={icon} size={20} color={active ? colors.foreground : colors.muted} />
    </Pressable>
  );
}

function MenuItem({
  icon,
  label,
  onPress,
  tone = 'default',
}: {
  icon: string;
  label: string;
  onPress: () => void;
  tone?: 'default' | 'danger';
}) {
  const colors = useColors();
  return (
    <Pressable onPress={onPress} className="flex-row items-center gap-3 px-4 py-3 active:bg-surface-alt">
      <Ionicon name={icon} size={18} color={tone === 'danger' ? colors.danger : colors.foreground} />
      <Text className={`text-sm ${tone === 'danger' ? 'text-danger' : 'text-foreground'}`}>{label}</Text>
    </Pressable>
  );
}
