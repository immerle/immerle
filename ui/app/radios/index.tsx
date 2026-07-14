import { useMemo } from 'react';
import { Pressable, ScrollView, Text, useWindowDimensions, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Stack, router } from 'expo-router';
import { useRadioStations } from '../../src/query/radio';
import { useAuth } from '../../src/auth/store';
import { EmptyState, ErrorState, IconButton, Loading } from '../../src/components/ui';
import { HeroBackdrop } from '../../src/components/HeroBackdrop';
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
  const { width } = useWindowDimensions();
  const wide = width >= 640;
  const insets = useSafeAreaInsets();
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
  const coverStation = (q.data ?? []).find((s) => s.hasCover);
  const heroUrl = coverStation && client ? client.radioCoverUrl(coverStation.id) : undefined;

  const Hero = (
    <HeroBackdrop url={heroUrl} tint={colors.primary} height={wide ? 170 : 150 + insets.top}>
      {!wide ? (
        <View className="absolute left-4 z-10" style={{ top: insets.top + 8 }}>
          <IconButton name="chevron-back" size={24} color="#fff" onPress={() => router.back()} accessibilityLabel={t('components.admin.back')} />
        </View>
      ) : null}
      <View className="px-4 pb-3">
        <Text numberOfLines={1} className={`font-extrabold tracking-tight text-white ${wide ? 'text-4xl' : 'text-3xl'}`}>
          {t('radio.title')}
        </Text>
        <Text className="pt-1 text-sm text-white/90">{t('radio.tabSubtitle')}</Text>
      </View>
    </HeroBackdrop>
  );

  if (!client?.isFeatureEnabled('internetRadio')) {
    return (
      <>
        <Stack.Screen options={{ headerShown: false }} />
        <View className="flex-1 bg-background">
          {Hero}
          <EmptyState icon="radio-outline" title={t('radio.unavailableTitle')} subtitle={t('radio.unavailableSubtitle')} />
        </View>
      </>
    );
  }
  if (q.isLoading) {
    return (
      <>
        <Stack.Screen options={{ headerShown: false }} />
        <Loading />
      </>
    );
  }
  if (q.isError) {
    return (
      <>
        <Stack.Screen options={{ headerShown: false }} />
        <ErrorState message={t('radio.loadError')} onRetry={q.refetch} />
      </>
    );
  }

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <View className="flex-1 bg-background">
        <ScrollView contentContainerStyle={{ paddingBottom: 16 }}>
          {Hero}
          <View className="flex-row flex-wrap gap-2.5 px-4 pt-4">
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
        </ScrollView>
      </View>
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
