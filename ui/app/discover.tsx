import { Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { FlashList } from '@shopify/flash-list';
import { usePublicPlaylists, useSubscriptionMutations } from '../src/query/playlists';
import { useAuth } from '../src/auth/store';
import { Button, EmptyState, ErrorState, Loading } from '../src/components/ui';
import { PlaylistMosaic } from '../src/components/PlaylistMosaic';
import { PublicPlaylistDTO } from '../src/api/immerleApi';
import { formatCount } from '../src/utils/format';

/**
 * Discover public playlists. Subscriptions are opt-in: a public playlist only
 * joins your library once you subscribe — then it shows up like a normal
 * (read-only) playlist. Unsubscribing removes it from your library; the owner's
 * playlist is untouched.
 */
export default function Discover() {
  const client = useAuth((s) => s.client);
  const q = usePublicPlaylists();
  const { subscribe, unsubscribe } = useSubscriptionMutations();

  if (!client?.has('publicPlaylists')) {
    return (
      <>
        <Stack.Screen options={{ title: 'Playlists publiques' }} />
        <View className="flex-1 bg-background">
          <EmptyState icon="globe-outline" title="Indisponible" subtitle="Cette instance n'expose pas les playlists publiques." />
        </View>
      </>
    );
  }

  return (
    <>
      <Stack.Screen options={{ title: 'Playlists publiques' }} />
      <View className="flex-1 bg-background">
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message="Impossible de charger les playlists publiques." onRetry={q.refetch} />
        ) : !q.data?.length ? (
          <EmptyState icon="globe-outline" title="Aucune playlist publique" subtitle="Rien à découvrir pour l'instant." />
        ) : (
          <FlashList<PublicPlaylistDTO>
            data={q.data}
            keyExtractor={(p) => p.id ?? p.name ?? ''}
            estimatedItemSize={72}
            refreshing={q.isRefetching}
            onRefresh={q.refetch}
            ListHeaderComponent={
              <Text className="px-4 pb-1 pt-3 text-sm text-muted">
                Abonnez-vous pour ajouter une playlist à votre bibliothèque (lecture seule).
              </Text>
            }
            renderItem={({ item }) => {
              const subscribed = !!item.subscribed;
              return (
                <View className="flex-row items-center gap-3 px-4 py-2">
                  <PlaylistMosaic covers={item.coverArts ?? []} size={52} rounded="rounded-lg" fallbackIcon="list" />
                  <View className="flex-1">
                    <Text numberOfLines={1} className="text-base font-semibold text-foreground">
                      {item.name}
                    </Text>
                    <Text numberOfLines={1} className="text-sm text-muted">
                      par {item.owner ?? '—'} · {formatCount(item.songCount)} titres
                    </Text>
                  </View>
                  {subscribed ? (
                    <Button
                      title="Abonné"
                      size="sm"
                      variant="secondary"
                      icon="checkmark"
                      loading={unsubscribe.isPending}
                      onPress={() => item.id && unsubscribe.mutate(item.id)}
                    />
                  ) : (
                    <Button
                      title="S'abonner"
                      size="sm"
                      loading={subscribe.isPending}
                      onPress={() => item.id && subscribe.mutate(item.id)}
                    />
                  )}
                </View>
              );
            }}
            contentContainerStyle={{ paddingBottom: 16 }}
          />
        )}
      </View>
    </>
  );
}
