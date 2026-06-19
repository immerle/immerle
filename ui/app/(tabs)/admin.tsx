import { Pressable, ScrollView, Text, View } from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useAuth } from '../../src/auth/store';
import { useLibraryStats } from '../../src/query/admin';
import { EmptyState, SectionHeader } from '../../src/components/ui';
import { ADMIN_MAX_WIDTH, AdminHeader } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { CapabilityFeature } from '../../src/api/immerle/types';
import { formatBytes, formatCount } from '../../src/utils/format';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';

// Instance-wide capability flags advertised at `/capabilities` — read-only,
// shown here (admin) rather than in user settings since they describe the server.
const FEATURES: { key: CapabilityFeature; labelKey: string; icon: string }[] = [
  { key: 'immerleAuth', labelKey: 'home.admin.feature.immerleAuth', icon: 'key' },
  { key: 'dynamicProviders', labelKey: 'home.admin.feature.dynamicProviders', icon: 'cube' },
  { key: 'federation', labelKey: 'home.admin.feature.federation', icon: 'git-network' },
  { key: 'jam', labelKey: 'home.admin.feature.jam', icon: 'radio' },
  { key: 'collaborativePlaylists', labelKey: 'home.admin.feature.collaborativePlaylists', icon: 'people' },
  { key: 'publicPlaylists', labelKey: 'home.admin.feature.publicPlaylists', icon: 'globe' },
  { key: 'social', labelKey: 'home.admin.feature.social', icon: 'chatbubbles' },
];

interface AdminLink {
  href: string;
  icon: string;
  titleKey: string;
  subtitleKey: string;
  color: string;
  /** When set, hidden unless the instance advertises this. */
  requires?: 'dynamicProviders' | 'runtimeSettings';
}

const LINKS: AdminLink[] = [
  { href: '/admin/users', icon: 'people', titleKey: 'home.admin.link.users.title', subtitleKey: 'home.admin.link.users.subtitle', color: '#3b82f6' },
  { href: '/admin/scan', icon: 'refresh-circle', titleKey: 'home.admin.link.library.title', subtitleKey: 'home.admin.link.library.subtitle', color: '#f59e0b' },
  {
    href: '/admin/providers',
    icon: 'cube',
    titleKey: 'home.admin.link.providers.title',
    subtitleKey: 'home.admin.link.providers.subtitle',
    color: '#8b5cf6',
    requires: 'dynamicProviders',
  },
  {
    href: '/admin/settings',
    icon: 'options',
    titleKey: 'home.admin.link.settings.title',
    subtitleKey: 'home.admin.link.settings.subtitle',
    color: '#0ea5e9',
    requires: 'runtimeSettings',
  },
];

/**
 * Admin home. Only reachable by admins (the tab itself is role-gated). Sections
 * that depend on Immerle-only capabilities are hidden when the instance is a
 * plain Subsonic server, so admins never see dead ends.
 */
export default function Admin() {
  const t = useT();
  const colors = useColors();
  const client = useAuth((s) => s.client);
  const stats = useLibraryStats();

  if (!client?.isAdmin) {
    return (
      <SafeAreaView edges={['top']} className="flex-1 bg-background">
        <EmptyState icon="lock-closed" title={t('home.admin.restricted')} subtitle={t('home.admin.restrictedSubtitle')} />
      </SafeAreaView>
    );
  }

  const visibleLinks = LINKS.filter((l) => !l.requires || client.has(l.requires));

  return (
    <SafeAreaView edges={['top']} className="flex-1 bg-background">
      <ScrollView contentContainerStyle={{ paddingBottom: 32 }}>
        <AdminHeader
          color={colors.primary}
          title={t('home.admin.title')}
          subtitle={client.serverUrl}
          showBack={false}
          trailing={
            <View className="rounded-full bg-success/15 px-2.5 py-1">
              <Text className="text-xs font-semibold text-success">{t('home.admin.badge')}</Text>
            </View>
          }
        />

        <View style={{ maxWidth: ADMIN_MAX_WIDTH, width: '100%', alignSelf: 'center' }}>
        {/* Overview stats */}
        <SectionHeader title={t('home.admin.overview')} />
        <View className="flex-row flex-wrap gap-2.5 px-4">
          <StatTile icon="people" color="#3b82f6" label={t('home.admin.artists')} value={formatCount(stats.data?.artistCount)} />
          <StatTile icon="albums" color="#8b5cf6" label={t('home.admin.albums')} value={formatCount(stats.data?.albumCount)} />
          <StatTile icon="musical-notes" color="#1ed760" label={t('home.admin.tracks')} value={formatCount(stats.data?.songCount)} />
          <StatTile icon="server" color="#f59e0b" label={t('home.admin.storage')} value={stats.data ? formatBytes(stats.data.totalSize) : '—'} />
        </View>

        {/* Management grid */}
        <SectionHeader title={t('home.admin.management')} />
        <View className="flex-row flex-wrap gap-2.5 px-4">
          {visibleLinks.map((l) => (
            <ManageTile key={l.href} link={l} onPress={() => router.push(l.href as never)} />
          ))}
        </View>

        {/* Instance capabilities (read-only, server-detected) */}
        <SectionHeader title={t('home.admin.instanceFeatures')} />
        <View className="flex-row flex-wrap gap-2 px-4">
          {FEATURES.map((f) => (
            <FeaturePill key={f.key} icon={f.icon} label={t(f.labelKey)} on={client.capabilities.features[f.key]} />
          ))}
        </View>
        <Text className="px-5 pt-2.5 text-xs text-muted">
          {t('home.admin.autoDetected', { version: client.capabilities.version })}
        </Text>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

function StatTile({ icon, color, label, value }: { icon: string; color: string; label: string; value: string }) {
  return (
    <View className="min-w-[46%] flex-1 flex-row items-center gap-3 rounded-2xl bg-surface p-3.5">
      <View className="h-10 w-10 items-center justify-center rounded-xl" style={{ backgroundColor: color + '22' }}>
        <Ionicon name={icon} size={20} color={color} />
      </View>
      <View className="flex-1">
        <Text className="text-xl font-bold text-foreground" numberOfLines={1}>
          {value}
        </Text>
        <Text className="text-xs text-muted">{label}</Text>
      </View>
    </View>
  );
}

function ManageTile({ link, onPress }: { link: AdminLink; onPress: () => void }) {
  const t = useT();
  const colors = useColors();
  return (
    <Pressable onPress={onPress} className="min-w-[46%] flex-1 active:opacity-80">
      <View className="h-full gap-3 rounded-2xl bg-surface p-4">
        <View className="flex-row items-center justify-between">
          <View className="h-11 w-11 items-center justify-center rounded-xl" style={{ backgroundColor: link.color + '22' }}>
            <Ionicon name={link.icon} size={22} color={link.color} />
          </View>
          <Ionicon name="chevron-forward" size={18} color={colors.muted} />
        </View>
        <View>
          <Text className="text-base font-semibold text-foreground">{t(link.titleKey)}</Text>
          <Text className="text-xs text-muted" numberOfLines={2}>
            {t(link.subtitleKey)}
          </Text>
        </View>
      </View>
    </Pressable>
  );
}

function FeaturePill({ icon, label, on }: { icon: string; label: string; on: boolean }) {
  const colors = useColors();
  return (
    <View
      className={`flex-row items-center gap-1.5 rounded-full px-3 py-1.5 ${on ? 'bg-primary/15' : 'bg-surface'}`}
      style={on ? undefined : { opacity: 0.6 }}
    >
      <Ionicon name={on ? icon : 'remove-circle-outline'} size={14} color={on ? colors.primary : colors.muted} />
      <Text className={`text-xs font-medium ${on ? 'text-primary' : 'text-muted'}`}>{label}</Text>
    </View>
  );
}
