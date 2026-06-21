import { Pressable, ScrollView, Text, View } from 'react-native';
import { router, usePathname } from 'expo-router';
import { useAuth } from '../auth/store';
import { Ionicon } from './Ionicon';
import { ADMIN_LINKS, AdminLink } from '../nav/adminLinks';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

const WIDTH = 248;

/**
 * Desktop admin sidebar. Replaces the library/playlist sidebar while inside the
 * admin section, turning the left rail into a page-nav menu (immich-style):
 * one row per management destination, the active one highlighted. There is no
 * overview entry (/admin redirects to the first section). Capability-gated
 * links hide on plain Subsonic instances.
 */
export function AdminSidebar() {
  const t = useT();
  const colors = useColors();
  const pathname = usePathname();
  const client = useAuth((s) => s.client);

  const visible = ADMIN_LINKS.filter((l) => !l.requires || (client?.has(l.requires) ?? false));

  return (
    <View style={{ width: WIDTH }} className="border-r border-border bg-surface">
      <View className="flex-row items-center gap-2 px-4 py-3">
        <Ionicon name="shield-checkmark" size={20} color={colors.primary} />
        <Text className="text-base font-bold text-foreground">{t('home.admin.title')}</Text>
      </View>

      <ScrollView contentContainerStyle={{ paddingHorizontal: 8, paddingBottom: 12 }}>
        {visible.map((l: AdminLink) => (
          <Row
            key={l.href}
            icon={l.icon}
            color={l.color}
            title={t(l.titleKey)}
            active={pathname === l.href}
            onPress={() => router.push(l.href as never)}
          />
        ))}
      </ScrollView>
    </View>
  );
}

function Row({
  icon,
  color,
  title,
  active,
  onPress,
}: {
  icon: string;
  color: string;
  title: string;
  active: boolean;
  onPress: () => void;
}) {
  return (
    <Pressable
      onPress={onPress}
      accessibilityLabel={title}
      className={`flex-row items-center gap-3 rounded-lg px-2.5 py-2 ${active ? 'bg-surface-alt' : ''} active:bg-surface-alt`}
    >
      <View className="h-9 w-9 items-center justify-center rounded-xl" style={{ backgroundColor: color + '22' }}>
        <Ionicon name={icon} size={18} color={color} />
      </View>
      <Text numberOfLines={1} className={`flex-1 text-sm font-medium ${active ? 'text-primary' : 'text-foreground'}`}>
        {title}
      </Text>
    </Pressable>
  );
}
