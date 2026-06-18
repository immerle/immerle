import { Text, useWindowDimensions, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useAlbum } from '../../src/query/library';
import { useAuth } from '../../src/auth/store';
import { CoverArt } from '../../src/components/CoverArt';
import { HeroBackdrop } from '../../src/components/HeroBackdrop';
import { TrackList } from '../../src/components/TrackList';
import { ErrorState, IconButton, Loading } from '../../src/components/ui';
import { PlayButton } from '../../src/components/PlayButton';
import { usePlayer } from '../../src/audio/store';
import { useColors } from '../../src/theme/colors';
import { Song } from '../../src/api/subsonic/types';
import { formatDuration } from '../../src/utils/format';
import { useT } from '../../src/i18n/store';

/** Album detail: an immersive Spotify-style hero + virtualized track list. */
export default function AlbumDetail() {
  const t = useT();
  const colors = useColors();
  const { width } = useWindowDimensions();
  const wide = width >= 640;
  const { id } = useLocalSearchParams<{ id: string }>();
  const client = useAuth((s) => s.client);
  const q = useAlbum(id);
  const playSongs = usePlayer((s) => s.playSongs);

  if (q.isLoading) return <Loading />;
  if (q.isError || !q.data) {
    return <ErrorState message={t('media.album.notFound')} onRetry={q.refetch} />;
  }

  const album = q.data;
  const songs = album.song ?? [];
  const totalDuration = songs.reduce((n, s) => n + (s.duration ?? 0), 0);
  const coverUrl = client?.subsonic.coverArtUrl(album.coverArt, 700);

  const playAll = () => songs.length && playSongs(songs, 0);
  const shuffle = () => songs.length && playSongs(shuffleArray(songs), 0);

  const Header = (
    <View>
      <HeroBackdrop url={coverUrl} height={wide ? 300 : 360}>
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

      {/* Action bar over the page background. */}
      <View className="flex-row items-center gap-5 px-4 py-4">
        <PlayButton onPress={playAll} size={56} accessibilityLabel={t('media.album.play')} />
        <IconButton name="shuffle" size={26} color={colors.muted} onPress={shuffle} accessibilityLabel={t('media.album.shuffle')} />
      </View>
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ title: album.name }} />
      <View className="flex-1 bg-background">
        <TrackList songs={songs} showArtwork={false} header={Header} refreshing={q.isRefetching} onRefresh={q.refetch} />
      </View>
    </>
  );
}

function shuffleArray(songs: Song[]): Song[] {
  const arr = [...songs];
  for (let i = arr.length - 1; i > 0; i -= 1) {
    const j = Math.floor(Math.random() * (i + 1));
    [arr[i], arr[j]] = [arr[j], arr[i]];
  }
  return arr;
}
