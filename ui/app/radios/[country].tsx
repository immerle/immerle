import { useMemo } from 'react';
import { Pressable, Text, View } from 'react-native';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useRadioStations, useRadioLike } from '../../src/query/radio';
import { useAuth } from '../../src/auth/store';
import { usePlayer } from '../../src/audio/store';
import { EmptyState, ErrorState, IconButton, Loading } from '../../src/components/ui';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { StationCover } from '../../src/components/StationCover';
import { Ionicon } from '../../src/components/Ionicon';
import { RadioStation } from '../../src/api/immerle/types';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';

/** Stations of one country group, or the user's favorites when country=favorites. */
export default function CountryRadios() {
  const t = useT();
  const colors = useColors();
  const { country } = useLocalSearchParams<{ country: string }>();
  const client = useAuth((s) => s.client);
  const playRadio = usePlayer((s) => s.playRadio);
  const like = useRadioLike();
  const q = useRadioStations();

  const isFav = country === 'favorites';
  const stations = useMemo(
    () => (q.data ?? []).filter((s) => (isFav ? s.liked : (s.country || 'int') === country)),
    [q.data, country, isFav],
  );
  const title = isFav ? t('radio.favorites') : t(`radio.country.${country}`);

  if (q.isLoading) return <Loading />;
  if (q.isError) return <ErrorState message={t('radio.loadError')} onRetry={q.refetch} />;

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll header={<AdminHeader color={colors.primary} title={title} subtitle={t('radio.stationCount', { count: stations.length })} showBack />}>
        {!stations.length ? (
          <EmptyState icon={isFav ? 'heart-outline' : 'radio-outline'} title={isFav ? t('radio.noFavorites') : t('radio.emptyTitle')} subtitle={isFav ? t('radio.noFavoritesSubtitle') : t('radio.emptySubtitle')} />
        ) : (
          stations.map((s: RadioStation) => (
            <Pressable
              key={s.id}
              onPress={() => playRadio({ id: s.id, name: s.name, streamUrl: s.streamUrl })}
              className="flex-row items-center gap-3 rounded-xl bg-surface px-3 py-2 active:opacity-70"
            >
              <StationCover uri={s.hasCover && client ? client.radioCoverUrl(s.id) : undefined} size={48} rounded={8} />
              <View className="flex-1">
                <Text numberOfLines={1} className="text-base font-semibold text-foreground">{s.name}</Text>
                <Text numberOfLines={1} className="text-xs text-muted">{s.homepageUrl?.replace(/^https?:\/\//, '') ?? ''}</Text>
              </View>
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
      </AdminScroll>
    </>
  );
}
