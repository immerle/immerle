import { Text, useWindowDimensions, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useUserHallOfFame } from '../../src/query/hallOfFame';
import { HeroBackdrop } from '../../src/components/HeroBackdrop';
import { HallOfFamePodium } from '../../src/components/HallOfFamePodium';
import { TrackList } from '../../src/components/TrackList';
import { EmptyState, ErrorState, IconButton, Loading } from '../../src/components/ui';
import { PlayButton } from '../../src/components/PlayButton';
import { usePlayer } from '../../src/audio/store';
import { useAuth } from '../../src/auth/store';
import { formatDuration } from '../../src/utils/format';
import { useT } from '../../src/i18n/store';
import { useWebTitle } from '../../src/utils/documentTitle';

/** Read-only view of another user's Hall of Fame — the profile page's "see
 * all" link. Editing (reorder, notes) stays on the caller's own /halloffame. */
export default function UserHallOfFameScreen() {
  const t = useT();
  const { width } = useWindowDimensions();
  const wide = width >= 640;
  const insets = useSafeAreaInsets();
  const client = useAuth((s) => s.client);
  const { username } = useLocalSearchParams<{ username: string }>();
  const q = useUserHallOfFame(username ?? '');
  const playSongs = usePlayer((s) => s.playSongs);

  useWebTitle(t('media.hallOfFame.title'));

  if (q.isLoading) {
    return (
      <>
        <Stack.Screen options={{ headerShown: false }} />
        <Loading />
      </>
    );
  }
  if (q.isError || !q.data) {
    return (
      <>
        <Stack.Screen options={{ headerShown: false }} />
        <ErrorState message={t('media.hallOfFame.loadError')} onRetry={q.refetch} />
      </>
    );
  }

  const songs = q.data.entries;
  const totalDuration = songs.reduce((n, s) => n + (s.duration ?? 0), 0);
  const coverUrl = client?.coverArtUrl(songs[0]?.coverArt, 700);

  const Header = (
    <View>
      <HeroBackdrop url={coverUrl} height={wide ? 260 : 300 + insets.top}>
        {!wide ? (
          <View className="absolute left-4 z-10" style={{ top: insets.top + 8 }}>
            <IconButton name="chevron-back" size={24} color="#fff" onPress={() => router.back()} accessibilityLabel={t('components.admin.back')} />
          </View>
        ) : null}
        <View className={`pb-5 ${wide ? 'flex-row items-end justify-between gap-6 pl-4 pr-10' : 'items-center gap-4 px-4'}`}>
          <View className={wide ? 'min-w-0' : 'items-center'}>
            <Text
              numberOfLines={2}
              className={`font-extrabold tracking-tight text-white ${wide ? 'text-5xl' : 'text-center text-3xl'}`}
            >
              {t('media.hallOfFame.title')}
            </Text>
            <Text className={`pt-3 text-sm text-white/90 ${wide ? '' : 'text-center'}`}>
              {t('media.hallOfFame.trackCount', { count: songs.length })} · {formatDuration(totalDuration)}
            </Text>
          </View>
          {songs.length ? <HallOfFamePodium top={songs.slice(0, 3)} onPress={(i) => playSongs(songs, i)} /> : null}
        </View>
      </HeroBackdrop>

      <View className="flex-row items-center px-4 py-4">
        <PlayButton
          onPress={() => songs.length && playSongs(songs, 0)}
          size={56}
          accessibilityLabel={t('media.hallOfFame.play')}
        />
      </View>
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <View className="flex-1 bg-background">
        {!songs.length ? (
          <>
            {Header}
            <EmptyState icon="trophy-outline" title={t('media.hallOfFame.empty')} subtitle={t('media.hallOfFame.emptySubtitle')} />
          </>
        ) : (
          <TrackList songs={songs} header={Header} showRank />
        )}
      </View>
    </>
  );
}
