import { ScrollView, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useAlbumList, useStarred } from '../../src/query/library';
import { useAuth } from '../../src/auth/store';
import { AlbumTile } from '../../src/components/AlbumCard';
import { Loading, SectionHeader } from '../../src/components/ui';
import { Album } from '../../src/api/subsonic/types';

const TILE = 150;

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
          <Text className="text-sm text-muted">Bonjour</Text>
          <Text className="text-3xl font-bold text-foreground">{displayName ?? 'Mélomane'}</Text>
        </View>

        {loading ? (
          <Loading />
        ) : (
          <>
            <AlbumRow title="Écoutés récemment" albums={recentlyPlayed.data} />
            <AlbumRow title="Les plus écoutés" albums={frequent.data} />
            {starred.data?.album?.length ? <AlbumRow title="Vos favoris" albums={starred.data.album} /> : null}
            <AlbumRow title="Au hasard" albums={random.data} />
          </>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}
