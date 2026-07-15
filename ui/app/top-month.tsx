import { View, Text } from 'react-native';
import { Stack } from 'expo-router';
import { useTopTracks } from '../src/query/library';
import { TrackList } from '../src/components/TrackList';
import { TopMonthCover } from '../src/components/TopMonthCover';
import { PlayButton } from '../src/components/PlayButton';
import { EmptyState, ErrorState, IconButton, Loading } from '../src/components/ui';
import { usePlayer } from '../src/audio/store';
import { Song } from '../src/api/subsonic/types';
import { formatDuration } from '../src/utils/format';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

function shuffleArray(songs: Song[]): Song[] {
  const a = [...songs];
  for (let i = a.length - 1; i > 0; i -= 1) {
    const j = Math.floor(Math.random() * (i + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}

/**
 * "Top du mois" — a virtual playlist of the caller's most-played tracks this
 * calendar month, computed live from scrobbles (same principle as Wrapped).
 * Read-only: play / shuffle, no CRUD.
 */
export default function TopMonth() {
  const t = useT();
  const colors = useColors();
  const q = useTopTracks('month');
  const playSongs = usePlayer((s) => s.playSongs);

  const songs = q.data ?? [];
  const totalDuration = songs.reduce((n, s) => n + (s.duration ?? 0), 0);

  const Header = (
    <View className="w-full max-w-2xl items-center gap-3 self-center px-4 pb-2 pt-2">
      <TopMonthCover size={200} rounded={16} />
      <Text className="text-2xl font-bold tracking-tight text-foreground">{t('media.topMonth.title')}</Text>
      <Text className="text-xs text-muted">
        {t('media.topMonth.trackCount', { count: songs.length })} · {formatDuration(totalDuration)}
      </Text>
      <View className="w-full flex-row items-center justify-between py-2">
        <IconButton
          name="shuffle"
          size={26}
          color={colors.muted}
          onPress={() => songs.length && playSongs(shuffleArray(songs), 0)}
          accessibilityLabel={t('media.topMonth.shuffle')}
        />
        <PlayButton
          onPress={() => songs.length && playSongs(songs, 0)}
          size={56}
          accessibilityLabel={t('media.topMonth.play')}
        />
      </View>
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ title: t('media.topMonth.title') }} />
      <View className="flex-1 bg-background">
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message={t('media.topMonth.loadError')} onRetry={q.refetch} />
        ) : songs.length === 0 ? (
          <View className="flex-1">
            {Header}
            <EmptyState icon="trending-up-outline" title={t('media.topMonth.empty')} subtitle={t('media.topMonth.emptySubtitle')} />
          </View>
        ) : (
          <TrackList songs={songs} header={Header} refreshing={q.isRefetching} onRefresh={q.refetch} />
        )}
      </View>
    </>
  );
}
