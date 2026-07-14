import { useMemo } from 'react';
import { Pressable, ScrollView, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { router } from 'expo-router';
import { useAlbumList, useStarred } from '../../src/query/library';
import { useAuth } from '../../src/auth/store';
import { usePlayer } from '../../src/audio/store';
import { useDownloads, OfflineEntry } from '../../src/offline/store';
import { AlbumTile } from '../../src/components/AlbumCard';
import { CoverArt } from '../../src/components/CoverArt';
import { Ionicon } from '../../src/components/Ionicon';
import { Button, Card, Loading, SectionHeader } from '../../src/components/ui';
import { useColors } from '../../src/theme/colors';
import { Album, Song } from '../../src/api/subsonic/types';
import { useT } from '../../src/i18n/store';

const TILE = 150;

/** Shortcut chip: background close to the page (subtle surface tint), icon/label in the accent color. */
function ShortcutChip({ icon, label, onPress }: { icon: string; label: string; onPress: () => void }) {
  const colors = useColors();
  return (
    <Pressable onPress={onPress} className="flex-row items-center gap-2 rounded-full bg-surface-alt px-4 py-2 active:opacity-70">
      <Ionicon name={icon} size={16} color={colors.foreground} />
      <Text className="text-sm font-semibold text-foreground">{label}</Text>
    </Pressable>
  );
}

/** Quick access to the "fake playlists" (liked/local/hall of fame/etc.) — same
 * destinations as the Playlists tab, offered here too since Home is the
 * default landing screen. */
function QuickAccessRow() {
  const t = useT();
  const canSmart = useAuth((s) => s.client?.isFeatureEnabled('smartPlaylists') ?? false);
  const canRadio = useAuth((s) => s.client?.isFeatureEnabled('internetRadio') ?? false);
  const canHallOfFame = useAuth((s) => s.client?.isFeatureEnabled('hallOfFame') ?? false);

  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8, paddingLeft: 12, paddingRight: 16 }}>
      <ShortcutChip icon="heart" label={t('home.playlists.likedTracks')} onPress={() => router.push('/liked' as never)} />
      <ShortcutChip icon="cloud-upload" label={t('components.sidebar.localSongs')} onPress={() => router.push('/local' as never)} />
      {canHallOfFame ? (
        <ShortcutChip icon="trophy" label={t('components.sidebar.hallOfFame')} onPress={() => router.push('/halloffame' as never)} />
      ) : null}
      {canSmart ? (
        <ShortcutChip icon="sparkles" label={t('smart.title')} onPress={() => router.push('/smart-playlists' as never)} />
      ) : null}
      {canRadio ? <ShortcutChip icon="radio" label={t('radio.title')} onPress={() => router.push('/radios' as never)} /> : null}
    </ScrollView>
  );
}

/** Shown on Home when the server can't be reached, with a retry button —
 * the tracks actually available offline are listed separately below. */
function OfflineBanner({ onRetry, retrying }: { onRetry: () => void; retrying: boolean }) {
  const t = useT();
  const colors = useColors();
  return (
    <View className="px-4 pb-2">
      <Card className="flex-row items-center gap-3">
        <Ionicon name="cloud-offline-outline" size={22} color={colors.muted} />
        <Text className="flex-1 text-base font-semibold text-foreground">{t('home.home.offlineTitle')}</Text>
        <Button title={t('home.home.retry')} variant="secondary" size="sm" loading={retrying} onPress={onRetry} />
      </Card>
    </View>
  );
}

/** An offline entry carries enough to play it (the file is resolved locally). */
function toSong(e: OfflineEntry): Song {
  return { id: e.id, title: e.title, artist: e.artist, album: e.album, coverArt: e.coverArt, duration: e.duration };
}

/** Same carousel look as the online album rows, but built from whatever is
 * actually saved for offline playback. */
function OfflineTracksRow() {
  const t = useT();
  const entries = useDownloads((s) => s.entries);
  const list = useMemo(() => Object.values(entries).sort((a, b) => b.downloadedAt - a.downloadedAt), [entries]);
  if (!list.length) return null;

  const play = (index: number) => void usePlayer.getState().playSongs(list.map(toSong), index);

  return (
    <View>
      <SectionHeader title={t('home.home.availableOffline')} />
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: 16 }}>
        {list.map((e, i) => (
          <Pressable key={e.id} onPress={() => play(i)} style={{ width: TILE }} className="mr-3 active:opacity-70">
            <CoverArt coverArt={e.coverArt} size={TILE} rounded="rounded-xl" />
            <Text numberOfLines={1} className="mt-2 text-sm font-semibold text-foreground">
              {e.title}
            </Text>
            <Text numberOfLines={1} className="text-xs text-muted">
              {e.artist}
            </Text>
          </Pressable>
        ))}
      </ScrollView>
    </View>
  );
}

/** Horizontal album carousel. */
function AlbumRow({ title, albums }: { title: string; albums: Album[] | undefined }) {
  if (!albums || albums.length === 0) return null;
  return (
    <View>
      <SectionHeader title={title} />
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: 16 }}>
        {albums.map((a) => (
          <AlbumTile key={a.id} album={a} size={TILE} />
        ))}
      </ScrollView>
    </View>
  );
}

/**
 * Home / "for you" screen: what the user played recently up top, then
 * server-computed suggestions.
 */
export default function Home() {
  const t = useT();
  const displayName = useAuth((s) => s.displayName ?? s.client?.username);
  const recentlyPlayed = useAlbumList('recent');
  const frequent = useAlbumList('frequent');
  const random = useAlbumList('random');
  const starred = useStarred();

  const loading = recentlyPlayed.isLoading && frequent.isLoading && random.isLoading;
  // All three failing (rather than just one) is a reasonable proxy for "the
  // server is unreachable" — there's no dedicated connectivity check today.
  const offline = recentlyPlayed.isError && frequent.isError && random.isError;
  const retrying = recentlyPlayed.isFetching || frequent.isFetching || random.isFetching;
  const retry = () => {
    void recentlyPlayed.refetch();
    void frequent.refetch();
    void random.refetch();
  };

  return (
    <SafeAreaView edges={['top']} className="flex-1 bg-background">
      <ScrollView contentContainerStyle={{ paddingBottom: 24 }}>
        <View className="px-4 pb-1 pt-3">
          <Text className="text-sm text-muted">{t('home.home.greeting')}</Text>
          <Text className="text-3xl font-bold text-foreground">{displayName ?? t('home.home.defaultName')}</Text>
        </View>

        <View className="pb-2 pt-1">
          <QuickAccessRow />
        </View>

        {offline ? (
          <>
            <OfflineBanner onRetry={retry} retrying={retrying} />
            <OfflineTracksRow />
          </>
        ) : loading ? (
          <Loading />
        ) : (
          <>
            <AlbumRow title={t('home.home.recentlyPlayed')} albums={recentlyPlayed.data} />
            <AlbumRow title={t('home.home.mostPlayed')} albums={frequent.data} />
            {starred.data?.album?.length ? <AlbumRow title={t('home.home.favorites')} albums={starred.data.album} /> : null}
            <AlbumRow title={t('home.home.random')} albums={random.data} />
          </>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}
