import { Text, useWindowDimensions, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useAlbum } from '../../src/query/library';
import { useAuth } from '../../src/auth/store';
import { useOfflineCatalog } from '../../src/offline/catalog';
import { CoverArt } from '../../src/components/CoverArt';
import { HeroBackdrop } from '../../src/components/HeroBackdrop';
import { TrackList } from '../../src/components/TrackList';
import { ErrorState, IconButton, Skeleton } from '../../src/components/ui';
import { PlayButton } from '../../src/components/PlayButton';
import { DownloadButton } from '../../src/components/DownloadButton';
import { usePlayer } from '../../src/audio/store';
import { useColors } from '../../src/theme/colors';
import { formatDuration } from '../../src/utils/format';
import { useT } from '../../src/i18n/store';
import { useWebTitle } from '../../src/utils/documentTitle';

/** Album detail: an immersive Spotify-style hero + virtualized track list. */
export default function AlbumDetail() {
  const t = useT();
  const colors = useColors();
  const { width } = useWindowDimensions();
  const wide = width >= 640;
  const insets = useSafeAreaInsets();
  const { id } = useLocalSearchParams<{ id: string }>();
  const client = useAuth((s) => s.client);
  const q = useAlbum(id);
  const cached = useOfflineCatalog((s) => s.albums[id]);
  const playSongs = usePlayer((s) => s.playSongs);
  const playShuffled = usePlayer((s) => s.playShuffled);
  const shuffleOn = usePlayer((s) => s.shuffle);
  useWebTitle(q.data?.name);

  if (q.isLoading && !cached) {
    return (
      <>
        <Stack.Screen options={{ headerShown: false }} />
        <View className="flex-1 bg-background">
          <HeroBackdrop height={wide ? 300 : 360 + insets.top}>
            {!wide ? (
              <View className="absolute left-4 z-10" style={{ top: insets.top + 8 }}>
                <IconButton name="chevron-back" size={24} color="#fff" onPress={() => router.back()} accessibilityLabel={t('components.admin.back')} />
              </View>
            ) : null}
            <View className={`px-4 pb-5 ${wide ? 'flex-row items-end gap-5' : 'items-center gap-3'}`}>
              <Skeleton style={{ width: wide ? 196 : 150, height: wide ? 196 : 150 }} />
              <View className={wide ? 'min-w-0 flex-1 gap-2' : 'items-center gap-2'}>
                <Skeleton style={{ width: wide ? 320 : 220, height: wide ? 44 : 30 }} />
                <Skeleton style={{ width: wide ? 200 : 160, height: 16 }} />
              </View>
            </View>
          </HeroBackdrop>
          <View className="gap-4 px-4 py-4">
            {Array.from({ length: 6 }).map((_, i) => (
              <View key={i} className="flex-row items-center gap-3">
                <Skeleton style={{ width: 20, height: 14 }} />
                <View className="flex-1 gap-2">
                  <Skeleton style={{ width: '70%', height: 14 }} />
                  <Skeleton style={{ width: '40%', height: 12 }} />
                </View>
              </View>
            ))}
          </View>
        </View>
      </>
    );
  }
  if ((q.isError || !q.data) && !cached) {
    return (
      <>
        <Stack.Screen options={{ headerShown: false }} />
        <ErrorState message={t('media.album.notFound')} onRetry={q.refetch} />
      </>
    );
  }

  // Prefer live data; fall back to the offline snapshot (from downloading the
  // whole album) when the server can't be reached.
  const album = q.data ?? { id: cached!.id, name: cached!.name, artist: cached!.artist, artistId: cached!.artistId, year: cached!.year, coverArt: cached!.coverArt, song: cached!.songs };
  const songs = album.song ?? [];
  const totalDuration = songs.reduce((n, s) => n + (s.duration ?? 0), 0);
  const coverUrl = client?.coverArtUrl(album.coverArt, 700);

  const playAll = () => songs.length && playSongs(songs, 0);
  const shuffle = () => playShuffled(songs);

  const Header = (
    <View>
      <HeroBackdrop url={coverUrl} height={wide ? 300 : 360 + insets.top}>
        {!wide ? (
          <View className="absolute left-4 z-10" style={{ top: insets.top + 8 }}>
            <IconButton name="chevron-back" size={24} color="#fff" onPress={() => router.back()} accessibilityLabel={t('components.admin.back')} />
          </View>
        ) : null}
        <View className={`px-4 pb-5 ${wide ? 'flex-row items-end gap-5' : 'items-center gap-3'}`}>
          <View className="rounded-md shadow-2xl" style={{ elevation: 8 }}>
            <CoverArt coverArt={album.coverArt} size={wide ? 196 : 150} rounded="rounded-md" />
          </View>
          <View className={wide ? 'min-w-0 flex-1' : 'items-center'}>
            <Text className="text-xs font-semibold uppercase tracking-wide text-white/80">{t('media.album.label')}</Text>
            <Text
              numberOfLines={2}
              className={`font-extrabold tracking-tight text-white ${wide ? 'text-5xl' : 'text-center text-3xl'}`}
            >
              {album.name}
            </Text>
            <Text className={`pt-3 text-sm text-white/90 ${wide ? '' : 'text-center'}`}>
              <Text
                className="font-bold"
                suppressHighlighting
                onPress={
                  album.artistId
                    ? () => router.push(`/artist/${album.artistId}` as never)
                    : undefined
                }
              >
                {album.artist}
              </Text>
              {album.year ? ` · ${album.year}` : ''} · {t('media.album.trackCount', { count: songs.length })} · {formatDuration(totalDuration)}
            </Text>
          </View>
        </View>
      </HeroBackdrop>

      <View className="flex-row items-center gap-5 px-4 py-4">
        <PlayButton onPress={playAll} size={56} accessibilityLabel={t('media.album.play')} />
        <IconButton name="shuffle" size={26} color={shuffleOn ? colors.primary : colors.muted} onPress={shuffle} accessibilityLabel={t('media.album.shuffle')} />
        <DownloadButton
          songs={songs}
          size={26}
          snapshot={{ type: 'album', id: album.id, name: album.name, artist: album.artist, artistId: album.artistId, year: album.year, coverArt: album.coverArt }}
        />
      </View>
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <View className="flex-1 bg-background">
        <TrackList songs={songs} showArtwork={false} header={Header} refreshing={q.isRefetching} onRefresh={q.refetch} />
      </View>
    </>
  );
}
