import { Pressable, Text, View } from 'react-native';
import { Stack, router } from 'expo-router';
import { FlashList } from '@shopify/flash-list';
import { useSmartPlaylists, useSmartPlaylistMutations } from '../src/query/smartPlaylists';
import { useAuth } from '../src/auth/store';
import { usePlayer } from '../src/audio/store';
import { Button, EmptyState, ErrorState, IconButton, Loading } from '../src/components/ui';
import { Ionicon } from '../src/components/Ionicon';
import { SmartPlaylist } from '../src/api/immerle/types';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

/**
 * Rule-based "smart" playlists: dynamic playlists that re-resolve to tracks
 * every time they're opened (e.g. "most played House", "added this month").
 */
export default function SmartPlaylists() {
  const t = useT();
  const colors = useColors();
  const client = useAuth((s) => s.client);
  const playSongs = usePlayer((s) => s.playSongs);
  const q = useSmartPlaylists();
  const { remove } = useSmartPlaylistMutations();

  if (!client?.has('smartPlaylists')) {
    return (
      <>
        <Stack.Screen options={{ title: t('smart.title') }} />
        <View className="flex-1 bg-background">
          <EmptyState icon="sparkles-outline" title={t('smart.unavailableTitle')} subtitle={t('smart.unavailableSubtitle')} />
        </View>
      </>
    );
  }

  const play = async (id: string) => {
    const songs = await client.getSmartPlaylistTracks(id);
    if (songs.length) playSongs(songs, 0);
  };

  return (
    <>
      <Stack.Screen
        options={{
          title: t('smart.title'),
          headerRight: () => (
            <IconButton name="add" color={colors.primary} onPress={() => router.push('/smart-playlist/new' as never)} accessibilityLabel={t('smart.new')} />
          ),
        }}
      />
      <View className="flex-1 bg-background">
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message={t('smart.loadError')} onRetry={q.refetch} />
        ) : !q.data?.length ? (
          <View className="flex-1 items-center justify-center gap-4">
            <EmptyState icon="sparkles-outline" title={t('smart.emptyTitle')} subtitle={t('smart.emptySubtitle')} />
            <Button title={t('smart.new')} icon="add" onPress={() => router.push('/smart-playlist/new' as never)} />
          </View>
        ) : (
          <FlashList<SmartPlaylist>
            data={q.data}
            keyExtractor={(p) => p.id}
            estimatedItemSize={68}
            refreshing={q.isRefetching}
            onRefresh={q.refetch}
            renderItem={({ item }) => (
              <View className="flex-row items-center gap-3 px-4 py-2">
                <View className="h-12 w-12 items-center justify-center rounded-lg bg-primary/15">
                  <Ionicon name="sparkles" size={22} color={colors.primary} />
                </View>
                <Pressable className="flex-1" onPress={() => router.push(`/smart-playlist/${item.id}` as never)}>
                  <Text numberOfLines={1} className="text-base font-semibold text-foreground">{item.name}</Text>
                  <Text numberOfLines={1} className="text-sm text-muted">
                    {t('smart.conditionCount', { count: item.rules?.conditions?.length ?? 0 })}
                  </Text>
                </Pressable>
                <IconButton name="play" color={colors.foreground} onPress={() => play(item.id)} accessibilityLabel={t('smart.play')} />
                <IconButton name="trash-outline" color={colors.danger} onPress={() => remove.mutate(item.id)} accessibilityLabel={t('smart.delete')} />
              </View>
            )}
            contentContainerStyle={{ paddingVertical: 8 }}
          />
        )}
      </View>
    </>
  );
}
