import { Pressable, Text, View } from 'react-native';
import { Stack, router } from 'expo-router';
import { FlashList } from '@shopify/flash-list';
import { usePublicPlaylists, useSubscriptionMutations } from '../src/query/playlists';
import { useAuth } from '../src/auth/store';
import { Button, EmptyState, ErrorState, Loading } from '../src/components/ui';
import { PlaylistCover } from '../src/components/PlaylistCover';
import { PublicPlaylistDTO } from '../src/api/immerleApi';
import { formatCount } from '../src/utils/format';
import { useT } from '../src/i18n/store';

/**
 * Discover public playlists. Subscribing adds a read-only copy to your library;
 * unsubscribing removes it without touching the owner's playlist.
 */
export default function Discover() {
  const t = useT();
  const client = useAuth((s) => s.client);
  const q = usePublicPlaylists();
  const { subscribe, unsubscribe } = useSubscriptionMutations();

  if (!client?.has('publicPlaylists')) {
    return (
      <>
        <Stack.Screen options={{ title: t('social.discover.title') }} />
        <View className="flex-1 bg-background">
          <EmptyState icon="globe-outline" title={t('social.discover.unavailableTitle')} subtitle={t('social.discover.unavailableSubtitle')} />
        </View>
      </>
    );
  }

  return (
    <>
      <Stack.Screen options={{ title: t('social.discover.title') }} />
      <View className="flex-1 bg-background">
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message={t('social.discover.loadError')} onRetry={q.refetch} />
        ) : !q.data?.length ? (
          <EmptyState icon="globe-outline" title={t('social.discover.emptyTitle')} subtitle={t('social.discover.emptySubtitle')} />
        ) : (
          <FlashList<PublicPlaylistDTO>
            data={q.data}
            keyExtractor={(p) => p.id ?? p.name ?? ''}
            estimatedItemSize={72}
            refreshing={q.isRefetching}
            onRefresh={q.refetch}
            ListHeaderComponent={
              <Text className="px-4 pb-1 pt-3 text-sm text-muted">
                {t('social.discover.subscribeHint')}
              </Text>
            }
            renderItem={({ item }) => {
              const subscribed = !!item.subscribed;
              return (
                <Pressable
                  onPress={() => item.id && router.push(`/playlist/${item.id}`)}
                  className="flex-row items-center gap-3 px-4 py-2 active:bg-surface-alt"
                >
                  <PlaylistCover coverArt={item.coverArt} covers={item.coverArts ?? []} size={52} rounded="rounded-lg" fallbackIcon="list" />
                  <View className="flex-1">
                    <Text numberOfLines={1} className="text-base font-semibold text-foreground">
                      {item.name}
                    </Text>
                    <Text numberOfLines={1} className="text-sm text-muted">
                      {t('social.discover.byOwner', { owner: item.owner ?? '—', count: formatCount(item.songCount) })}
                    </Text>
                  </View>
                  {subscribed ? (
                    <Button
                      title={t('social.discover.subscribed')}
                      size="sm"
                      variant="secondary"
                      icon="checkmark"
                      loading={unsubscribe.isPending}
                      onPress={() => item.id && unsubscribe.mutate(item.id)}
                    />
                  ) : (
                    <Button
                      title={t('social.discover.subscribe')}
                      size="sm"
                      loading={subscribe.isPending}
                      onPress={() => item.id && subscribe.mutate(item.id)}
                    />
                  )}
                </Pressable>
              );
            }}
            contentContainerStyle={{ paddingBottom: 16 }}
          />
        )}
      </View>
    </>
  );
}
