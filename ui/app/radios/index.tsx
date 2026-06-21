import { useMemo } from 'react';
import { Pressable, Text, View } from 'react-native';
import { Stack, router } from 'expo-router';
import { useRadioStations } from '../../src/query/radio';
import { useAuth } from '../../src/auth/store';
import { EmptyState, ErrorState, Loading } from '../../src/components/ui';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';

// Display order + flag per built-in country group.
const COUNTRY_ORDER = ['fr', 'es', 'gb', 'us', 'ch', 'int'];
const FLAG: Record<string, string> = { fr: '🇫🇷', es: '🇪🇸', gb: '🇬🇧', us: '🇺🇸', ch: '🇨🇭', int: '🌍' };

/** Radio home: pick a country (with a pinned "Favorites" card when any station
 * has been liked), then browse that country's stations. */
export default function RadiosHome() {
  const t = useT();
  const colors = useColors();
  const client = useAuth((s) => s.client);
  const q = useRadioStations();

  const { groups, likedCount } = useMemo(() => {
    const list = q.data ?? [];
    const counts: Record<string, number> = {};
    for (const s of list) {
      const cc = s.country || 'int';
      counts[cc] = (counts[cc] ?? 0) + 1;
    }
    const order = [...COUNTRY_ORDER, ...Object.keys(counts).filter((c) => !COUNTRY_ORDER.includes(c))];
    return {
      groups: order.filter((c) => counts[c]).map((c) => ({ code: c, count: counts[c] })),
      likedCount: list.filter((s) => s.liked).length,
    };
  }, [q.data]);

  if (!client?.has('internetRadio')) {
    return (
      <AdminScroll header={<AdminHeader color={colors.primary} title={t('radio.title')} showBack />}>
        <EmptyState icon="radio-outline" title={t('radio.unavailableTitle')} subtitle={t('radio.unavailableSubtitle')} />
      </AdminScroll>
    );
  }
  if (q.isLoading) return <Loading />;
  if (q.isError) return <ErrorState message={t('radio.loadError')} onRetry={q.refetch} />;

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll header={<AdminHeader color={colors.primary} title={t('radio.title')} subtitle={t('radio.tabSubtitle')} showBack />}>
        <View className="flex-row flex-wrap gap-2.5">
          {likedCount > 0 ? (
            <CountryCard
              emoji="❤️"
              label={t('radio.favorites')}
              count={likedCount}
              accent
              onPress={() => router.push('/radios/favorites' as never)}
            />
          ) : null}
          {groups.map((g) => (
            <CountryCard
              key={g.code}
              emoji={FLAG[g.code] ?? '🌍'}
              label={t(`radio.country.${g.code}`)}
              count={g.count}
              onPress={() => router.push(`/radios/${g.code}` as never)}
            />
          ))}
        </View>
        {!groups.length ? <EmptyState icon="radio-outline" title={t('radio.emptyTitle')} subtitle={t('radio.emptySubtitle')} /> : null}
      </AdminScroll>
    </>
  );
}

function CountryCard({ emoji, label, count, accent, onPress }: { emoji: string; label: string; count: number; accent?: boolean; onPress: () => void }) {
  const t = useT();
  const colors = useColors();
  return (
    <Pressable onPress={onPress} className="min-w-[46%] flex-1 active:opacity-80">
      <View className={`gap-2 rounded-2xl p-4 ${accent ? 'bg-primary/15' : 'bg-surface'}`}>
        <View className="flex-row items-center justify-between">
          <Text style={{ fontSize: 30 }}>{emoji}</Text>
          <Ionicon name="chevron-forward" size={18} color={colors.muted} />
        </View>
        <View>
          <Text className="text-base font-semibold text-foreground">{label}</Text>
          <Text className="text-xs text-muted">{t('radio.stationCount', { count })}</Text>
        </View>
      </View>
    </Pressable>
  );
}
