import { useState } from 'react';
import { ScrollView, Text, useWindowDimensions, View } from 'react-native';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useArtist } from '../../src/query/library';
import { useAuth } from '../../src/auth/store';
import { HeroBackdrop } from '../../src/components/HeroBackdrop';
import { AlbumCard } from '../../src/components/AlbumCard';
import { ErrorState, IconButton, Loading } from '../../src/components/ui';
import { PlayButton } from '../../src/components/PlayButton';
import { usePlayer } from '../../src/audio/store';
import { Song } from '../../src/api/subsonic/types';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';
import { useWebTitle } from '../../src/utils/documentTitle';

/** Artist detail: immersive hero + discography grid, with play/shuffle. */
export default function ArtistDetail() {
  const t = useT();
  const colors = useColors();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { width } = useWindowDimensions();
  const client = useAuth((s) => s.client);
  const q = useArtist(id);
  const playSongs = usePlayer((s) => s.playSongs);
  const [busy, setBusy] = useState(false);
  useWebTitle(q.data?.name);

  if (q.isLoading) return <Loading />;
  if (q.isError || !q.data) return <ErrorState message={t('media.artist.notFound')} onRetry={q.refetch} />;

  const artist = q.data;
  const albums = artist.album ?? [];
  const wide = width >= 640;
  const gap = 16;
  // Fixed, compact card size that wraps to fill the content area (window width
  // overestimates it on desktop because of the sidebar). Two columns on mobile.
  const cardWidth = wide ? 150 : Math.floor((width - gap * 3) / 2);
  // Prefer the artist photo; fall back to the first album's cover when the
  // artist has no image of its own.
  const heroUrl =
    artist.artistImageUrl ??
    client?.subsonic.coverArtUrl(artist.coverArt, 700) ??
    client?.subsonic.coverArtUrl(albums[0]?.coverArt, 700);

  // Artist-level play gathers tracks from every album on demand (no top-songs
  // data in plain Subsonic libraries).
  const gather = async (): Promise<Song[]> => {
    if (!client) return [];
    const results = await Promise.all(
      albums.map((a) => client.subsonic.getAlbum(a.id).catch(() => null)),
    );
    return results.flatMap((r) => r?.song ?? []);
  };
  const run = async (shuffle: boolean) => {
    if (busy || !albums.length) return;
    setBusy(true);
    try {
      const songs = await gather();
      if (songs.length) playSongs(shuffle ? shuffleArray(songs) : songs, 0);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Stack.Screen options={{ title: artist.name }} />
      <ScrollView className="flex-1 bg-background" contentContainerStyle={{ paddingBottom: 24 }}>
        <HeroBackdrop url={heroUrl} height={wide ? 170 : 150}>
          <View className="px-4 pb-3">
            <Text className="text-[11px] font-semibold uppercase tracking-wide text-white/80">{t('media.artist.label')}</Text>
            <Text
              numberOfLines={1}
              className={`font-extrabold tracking-tight text-white ${wide ? 'text-4xl' : 'text-3xl'}`}
            >
              {artist.name}
            </Text>
            <Text className="pt-1 text-sm text-white/90">
              {t('media.artist.albumCount', { count: albums.length })}
            </Text>
          </View>
        </HeroBackdrop>

        <View className="flex-row items-center gap-5 px-4 pb-2 pt-3">
          <PlayButton onPress={() => run(false)} size={56} accessibilityLabel={t('media.artist.play')} />
          <IconButton name="shuffle" size={26} color={busy ? colors.primary : colors.muted} onPress={() => run(true)} accessibilityLabel={t('media.artist.shuffle')} />
        </View>

        <Text className="px-4 pb-2 text-xl font-bold tracking-tight text-foreground">{t('media.artist.discography')}</Text>
        <View className="flex-row flex-wrap" style={{ paddingHorizontal: gap / 2 }}>
          {albums.map((a) => (
            <View key={a.id} style={{ paddingHorizontal: gap / 2 }}>
              <AlbumCard album={a} width={cardWidth} />
            </View>
          ))}
        </View>
      </ScrollView>
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
