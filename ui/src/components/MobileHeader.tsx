import { useState } from 'react';
import { Image, Modal, Pressable, Text, View } from 'react-native';
import { router } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Ionicon } from './Ionicon';
import { NotificationsBell } from './NotificationsBell';
import { useAuth } from '../auth/store';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

/**
 * Compact global header for narrow (mobile) layouts: the logo + wordmark on
 * the left, the account avatar on the right (search and library nav already
 * live in the bottom tab bar, so they aren't duplicated here). The avatar
 * opens the same Settings menu as the desktop top bar. Rendered
 * by the tabs layout so it sits above every main screen; per-screen titles
 * live underneath.
 */
export function MobileHeader() {
  const t = useT();
  const insets = useSafeAreaInsets();
  const client = useAuth((s) => s.client);
  const displayNameState = useAuth((s) => s.displayName);
  const [menu, setMenu] = useState(false);

  const displayName = displayNameState ?? client?.username ?? '?';
  const initial = displayName.charAt(0).toUpperCase();
  const hasSocial = client?.has('social') ?? false;

  const go = (href: string) => {
    setMenu(false);
    router.navigate(href as never);
  };
  return (
    <View
      className="flex-row items-center justify-between border-b border-border bg-background px-4"
      style={{ paddingTop: insets.top, height: 52 + insets.top }}
    >
      <Pressable onPress={() => go('/')} accessibilityRole="button" accessibilityLabel="Immerle" className="flex-row items-center gap-2 active:opacity-70">
        <Image source={require('../../assets/logo.png')} style={{ height: 28, width: 28 * (480 / 391) }} resizeMode="contain" />
        <Text className="text-lg font-bold text-[#333333] dark:text-white">Immerle</Text>
      </Pressable>

      <View className="flex-row items-center gap-2">
        <NotificationsBell />
        <Pressable
          onPress={() => setMenu(true)}
          accessibilityRole="button"
          accessibilityLabel={t('components.topbar.accountMenu')}
          className="ml-1 h-9 w-9 items-center justify-center rounded-full bg-primary active:opacity-80"
        >
          <Text className="text-sm font-bold text-primary-foreground">{initial}</Text>
        </Pressable>
      </View>

      <Modal transparent visible={menu} animationType="fade" onRequestClose={() => setMenu(false)}>
        <Pressable className="flex-1" onPress={() => setMenu(false)}>
          <View className="absolute right-3 w-56 overflow-hidden rounded-xl border border-border bg-surface" style={{ top: insets.top + 50 }}>
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
            {/* Administration is web/desktop-only — kept out of the mobile menu. */}
          </View>
        </Pressable>
      </Modal>
    </View>
  );
}

function MenuItem({ icon, label, onPress, tone = 'default' }: { icon: string; label: string; onPress: () => void; tone?: 'default' | 'danger' }) {
  const colors = useColors();
  return (
    <Pressable onPress={onPress} className="flex-row items-center gap-3 px-4 py-3 active:bg-surface-alt">
      <Ionicon name={icon} size={18} color={tone === 'danger' ? colors.danger : colors.foreground} />
      <Text className={`text-sm ${tone === 'danger' ? 'text-danger' : 'text-foreground'}`}>{label}</Text>
    </Pressable>
  );
}
