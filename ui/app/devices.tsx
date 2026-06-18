import { Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { useNowPlaying } from '../src/query/account';
import { Card, EmptyState, ErrorState, Loading } from '../src/components/ui';
import { AdminHeader, AdminScroll } from '../src/components/AdminUI';
import { CoverArt } from '../src/components/CoverArt';
import { Ionicon } from '../src/components/Ionicon';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

/**
 * Connected devices / active sessions, from the Subsonic `getNowPlaying`
 * endpoint: who is playing what, on which player, and how long ago.
 */
export default function Devices() {
  const t = useT();
  const colors = useColors();
  const q = useNowPlaying();

  const ago = (m?: number) => {
    if (m == null) return '';
    if (m <= 0) return t('tools.devices.justNow');
    if (m < 60) return t('tools.devices.minutesAgo', { count: m });
    return t('tools.devices.hoursAgo', { count: Math.floor(m / 60) });
  };

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll
        header={<AdminHeader color="#14b8a6" title={t('tools.devices.title')} subtitle={t('tools.devices.subtitle')} />}
      >
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message={t('tools.devices.loadError')} onRetry={q.refetch} />
        ) : !q.data?.length ? (
          <EmptyState icon="phone-portrait-outline" title={t('tools.devices.emptyTitle')} subtitle={t('tools.devices.emptySubtitle')} />
        ) : (
          q.data.map((e, i) => (
            <Card key={`${e.playerId ?? e.username ?? i}:${i}`} className="flex-row items-center gap-3">
              <CoverArt coverArt={e.coverArt} size={48} rounded="rounded-md" />
              <View className="flex-1">
                <View className="flex-row items-center gap-2">
                  <Ionicon name="hardware-chip-outline" size={16} color={colors.primary} />
                  <Text className="text-base font-semibold text-foreground">{e.playerName || e.username || t('tools.devices.fallbackName')}</Text>
                </View>
                <Text numberOfLines={1} className="text-sm text-muted">
                  {e.title}
                  {e.artist ? ` · ${e.artist}` : ''}
                </Text>
                <Text className="text-xs text-muted">
                  {e.username ? `${e.username} · ` : ''}
                  {ago(e.minutesAgo)}
                </Text>
              </View>
            </Card>
          ))
        )}
      </AdminScroll>
    </>
  );
}
