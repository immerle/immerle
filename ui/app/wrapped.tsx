import { useState } from 'react';
import { Pressable, ScrollView, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { useWrapped } from '../src/query/wrapped';
import { Card, EmptyState, ErrorState, Loading } from '../src/components/ui';
import { CardTitle } from '../src/components/AdminUI';
import { Ionicon } from '../src/components/Ionicon';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

const MONTH_KEYS = ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec'];

/**
 * "Wrapped" — the caller's year-in-review: totals, a per-month histogram, and
 * top tracks / artists / genres. Data comes from the scrobble history; the year
 * is switchable (bounded between 2000 and the current year).
 */
export default function WrappedScreen() {
  const t = useT();
  const colors = useColors();
  const now = new Date().getFullYear();
  const [year, setYear] = useState(now);
  const q = useWrapped(year);

  const YearNav = (
    <View className="flex-row items-center justify-center gap-6 py-3">
      <Pressable disabled={year <= 2000} onPress={() => setYear((y) => y - 1)} className="active:opacity-60" style={{ opacity: year <= 2000 ? 0.3 : 1 }}>
        <Ionicon name="chevron-back" size={24} color={colors.foreground} />
      </Pressable>
      <Text className="text-2xl font-bold text-foreground">{year}</Text>
      <Pressable disabled={year >= now} onPress={() => setYear((y) => y + 1)} className="active:opacity-60" style={{ opacity: year >= now ? 0.3 : 1 }}>
        <Ionicon name="chevron-forward" size={24} color={colors.foreground} />
      </Pressable>
    </View>
  );

  const w = q.data;
  const hours = w ? Math.round(w.totalSeconds / 3600) : 0;
  const maxMonth = w ? Math.max(1, ...w.byMonth) : 1;

  return (
    <>
      <Stack.Screen options={{ title: t('wrapped.title') }} />
      <ScrollView className="flex-1 bg-background" contentContainerStyle={{ padding: 16, gap: 12, paddingBottom: 32 }}>
        {YearNav}
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message={t('wrapped.loadError')} onRetry={q.refetch} />
        ) : !w || w.totalPlays === 0 ? (
          <EmptyState icon="sparkles-outline" title={t('wrapped.emptyTitle')} subtitle={t('wrapped.emptySubtitle')} />
        ) : (
          <>
            {/* Totals */}
            <View className="flex-row gap-3">
              <Stat label={t('wrapped.plays')} value={String(w.totalPlays)} />
              <Stat label={t('wrapped.hours')} value={String(hours)} />
            </View>

            {/* Per-month histogram */}
            <Card className="gap-3">
              <CardTitle icon="bar-chart" color={colors.primary} title={t('wrapped.byMonth')} />
              <View className="flex-row items-end justify-between" style={{ height: 96 }}>
                {w.byMonth.map((count, i) => (
                  <View key={i} className="flex-1 items-center gap-1">
                    <View
                      className="w-2.5 rounded-full"
                      style={{ height: Math.max(2, (count / maxMonth) * 72), backgroundColor: colors.primary }}
                    />
                    <Text className="text-[9px] text-muted">{t(`wrapped.month.${MONTH_KEYS[i]}`)}</Text>
                  </View>
                ))}
              </View>
            </Card>

            {/* Top tracks */}
            {w.topTracks?.length ? (
              <Card className="gap-2">
                <CardTitle icon="musical-notes" color="#ec4899" title={t('wrapped.topTracks')} />
                {w.topTracks.map((tr, i) => (
                  <Rank key={tr.id} n={i + 1} title={tr.title} subtitle={tr.artist} plays={tr.plays} playsLabel={t('wrapped.playsShort', { count: tr.plays })} />
                ))}
              </Card>
            ) : null}

            {/* Top artists */}
            {w.topArtists?.length ? (
              <Card className="gap-2">
                <CardTitle icon="person" color="#14b8a6" title={t('wrapped.topArtists')} />
                {w.topArtists.map((a, i) => (
                  <Rank key={a.name} n={i + 1} title={a.name} plays={a.plays} playsLabel={t('wrapped.playsShort', { count: a.plays })} />
                ))}
              </Card>
            ) : null}

            {/* Top genres */}
            {w.topGenres?.length ? (
              <Card className="gap-2">
                <CardTitle icon="pricetag" color="#f59e0b" title={t('wrapped.topGenres')} />
                {w.topGenres.map((g, i) => (
                  <Rank key={g.name} n={i + 1} title={g.name} plays={g.plays} playsLabel={t('wrapped.playsShort', { count: g.plays })} />
                ))}
              </Card>
            ) : null}
          </>
        )}
      </ScrollView>
    </>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <View className="flex-1 items-center rounded-2xl bg-surface-alt py-4">
      <Text className="text-3xl font-extrabold text-primary">{value}</Text>
      <Text className="text-sm text-muted">{label}</Text>
    </View>
  );
}

function Rank({ n, title, subtitle, plays, playsLabel }: { n: number; title: string; subtitle?: string; plays: number; playsLabel: string }) {
  return (
    <View className="flex-row items-center gap-3 py-1">
      <Text className="w-6 text-center text-base font-bold text-muted">{n}</Text>
      <View className="flex-1">
        <Text numberOfLines={1} className="text-base font-semibold text-foreground">{title}</Text>
        {subtitle ? <Text numberOfLines={1} className="text-sm text-muted">{subtitle}</Text> : null}
      </View>
      <Text className="text-sm font-medium text-muted">{playsLabel}</Text>
    </View>
  );
}
