import { useMemo } from 'react';
import { Pressable, ScrollView, Text, useWindowDimensions, View } from 'react-native';
import { SafeAreaView, useSafeAreaInsets } from 'react-native-safe-area-context';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useRadioStations, useRadioLike } from '../../src/query/radio';
import { useAuth } from '../../src/auth/store';
import { usePlayer } from '../../src/audio/store';
import { EmptyState, ErrorState, IconButton, Loading } from '../../src/components/ui';
import { HeroBackdrop } from '../../src/components/HeroBackdrop';
import { StationCover } from '../../src/components/StationCover';
import { Ionicon } from '../../src/components/Ionicon';
import { RadioStation } from '../../src/api/immerle/types';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';

/** Stations of one country group, or the user's favorites when country=favorites. */
export default function CountryRadios() {
  const t = useT();
  const colors = useColors();
  const { width } = useWindowDimensions();
  const wide = width >= 640;
  const insets = useSafeAreaInsets();
  const { country } = useLocalSearchParams<{ country: string }>();
  const client = useAuth((s) => s.client);
  const playRadio = usePlayer((s) => s.playRadio);
  const like = useRadioLike();
  const q = useRadioStations();

  const isFav = country === 'favorites';
  const stations = useMemo(
    () =>
      (q.data ?? [])
        .filter((s) => (isFav ? s.liked : (s.country || 'int') === country))
        .sort((a, b) => a.name.localeCompare(b.name)),
    [q.data, country, isFav],
  );
  const title = isFav ? t('radio.favorites') : t(`radio.country.${country}`);
  const coverStation = stations.find((s) => s.hasCover);
  const heroUrl = coverStation && client ? client.radioCoverUrl(coverStation.id) : undefined;

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
      <SafeAreaView edges={['top']} className="flex-1 bg-background">
        <ScrollView contentContainerStyle={{ paddingBottom: 16 }}>
          <HeroBackdrop url={heroUrl} height={wide ? 170 : 150 + insets.top}>
            {!wide ? (
              <View className="absolute left-4 z-10" style={{ top: insets.top + 8 }}>
                <IconButton name="chevron-back" size={24} color="#fff" onPress={() => router.back()} accessibilityLabel={t('components.admin.back')} />
              </View>
            ) : null}
            <View className="px-4 pb-3">
              <Text
                numberOfLines={1}
                className={`font-extrabold tracking-tight text-white ${wide ? 'text-4xl' : 'text-3xl'}`}
              >
                {title}
              </Text>
              <Text className="pt-1 text-sm text-white/90">{t('radio.stationCount', { count: stations.length })}</Text>
            </View>
          </HeroBackdrop>

          <View className="gap-2 px-4 pt-4">
            {!stations.length ? (
              <EmptyState icon={isFav ? 'heart-outline' : 'radio-outline'} title={isFav ? t('radio.noFavorites') : t('radio.emptyTitle')} subtitle={isFav ? t('radio.noFavoritesSubtitle') : t('radio.emptySubtitle')} />
            ) : (
              stations.map((s: RadioStation) => (
                <Pressable
                  key={s.id}
                  onPress={() => playRadio({ id: s.id, name: s.name, streamUrl: s.streamUrl, hasCover: s.hasCover })}
                  className="flex-row items-center gap-3 rounded-xl bg-surface px-3 py-2 active:opacity-70"
                >
                  <StationCover uri={s.hasCover && client ? client.radioCoverUrl(s.id) : undefined} size={48} rounded={8} />
                  <Text numberOfLines={1} className="flex-1 text-base font-semibold text-foreground">{s.name}</Text>
                  <IconButton
                    name={s.liked ? 'heart' : 'heart-outline'}
                    color={s.liked ? colors.primary : colors.muted}
                    onPress={() => like.mutate({ id: s.id, liked: !s.liked })}
                    accessibilityLabel={t(s.liked ? 'radio.unlike' : 'radio.like')}
                  />
                  <Ionicon name="play" size={22} color={colors.foreground} />
                </Pressable>
              ))
            )}
          </View>
        </ScrollView>
      </SafeAreaView>
    </>
  );
}
