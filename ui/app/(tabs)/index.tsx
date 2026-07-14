import { Pressable, ScrollView, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { router } from 'expo-router';
import { useAlbumList, useStarred } from '../../src/query/library';
import { useAuth } from '../../src/auth/store';
import { AlbumTile } from '../../src/components/AlbumCard';
import { Ionicon } from '../../src/components/Ionicon';
import { Loading, SectionHeader } from '../../src/components/ui';
import { useColors } from '../../src/theme/colors';
import { Album } from '../../src/api/subsonic/types';
import { useT } from '../../src/i18n/store';

const TILE = 150;

/** Shortcut chip: background close to the page (subtle surface tint), icon/label in the accent color. */
function ShortcutChip({ icon, label, onPress }: { icon: string; label: string; onPress: () => void }) {
  const colors = useColors();
  return (
    <Pressable onPress={onPress} className="flex-row items-center gap-2 rounded-full bg-surface-alt px-4 py-2 active:opacity-70">
      <Ionicon name={icon} size={16} color={colors.primary} />
      <Text className="text-sm font-semibold text-primary">{label}</Text>
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
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8, paddingHorizontal: 16 }}>
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

        {loading ? (
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
