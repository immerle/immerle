import { Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { useStarred } from '../src/query/library';
import { TrackList } from '../src/components/TrackList';
import { LikedCover } from '../src/components/LikedCover';
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
 * "Titres likés" — a virtual playlist backed by the Subsonic `getStarred2`
 * endpoint (starred songs). Read-only: play / shuffle, no CRUD.
 */
export default function Liked() {
  const t = useT();
  const colors = useColors();
  const q = useStarred();
  const playSongs = usePlayer((s) => s.playSongs);

  const songs = q.data?.song ?? [];
  const totalDuration = songs.reduce((n, s) => n + (s.duration ?? 0), 0);

  const Header = (
    <View className="w-full max-w-2xl items-center gap-3 self-center px-4 pb-2 pt-2">
      <LikedCover size={200} rounded={16} />
      <Text className="text-2xl font-bold tracking-tight text-foreground">{t('media.liked.title')}</Text>
      <Text className="text-xs text-muted">
        {t('media.liked.trackCount', { count: songs.length })} · {formatDuration(totalDuration)}
      </Text>
      <View className="w-full flex-row items-center justify-between py-2">
        <IconButton
          name="shuffle"
          size={26}
          color={colors.muted}
          onPress={() => songs.length && playSongs(shuffleArray(songs), 0)}
          accessibilityLabel={t('media.liked.shuffle')}
        />
        <PlayButton
          onPress={() => songs.length && playSongs(songs, 0)}
          size={56}
          accessibilityLabel={t('media.liked.play')}
        />
      </View>
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ title: t('media.liked.title') }} />
      <View className="flex-1 bg-background">
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message={t('media.liked.loadError')} onRetry={q.refetch} />
        ) : songs.length === 0 ? (
          <View className="flex-1">
            {Header}
            <EmptyState icon="heart-outline" title={t('media.liked.empty')} subtitle={t('media.liked.emptySubtitle')} />
          </View>
        ) : (
          <TrackList songs={songs} header={Header} refreshing={q.isRefetching} onRefresh={q.refetch} />
        )}
      </View>
    </>
  );
}
