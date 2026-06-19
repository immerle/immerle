import { View } from 'react-native';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useSongsByGenre } from '../../src/query/library';
import { TrackList } from '../../src/components/TrackList';
import { EmptyState, ErrorState, Loading } from '../../src/components/ui';
import { useT } from '../../src/i18n/store';
import { useWebTitle } from '../../src/utils/documentTitle';

/** Genre detail: a virtualized list of songs tagged with the genre. */
export default function GenreDetail() {
  const t = useT();
  const { id } = useLocalSearchParams<{ id: string }>();
  const genre = decodeURIComponent(id ?? '');
  const q = useSongsByGenre(genre);
  useWebTitle(genre);

  return (
    <>
      <Stack.Screen options={{ title: genre }} />
      <View className="flex-1 bg-background">
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message={t('media.genre.loadError')} onRetry={q.refetch} />
        ) : !q.data?.length ? (
          <EmptyState title={t('media.genre.empty')} />
        ) : (
          <TrackList songs={q.data} refreshing={q.isRefetching} onRefresh={q.refetch} />
        )}
      </View>
    </>
  );
}
