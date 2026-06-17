import { memo } from 'react';
import { Pressable, Text, View } from 'react-native';
import { router } from 'expo-router';
import { Artist } from '../api/subsonic/types';
import { CoverArt } from './CoverArt';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';

/** List row for an artist with a circular avatar. */
export const ArtistRow = memo(function ArtistRow({ artist }: { artist: Artist }) {
  const colors = useColors();
  return (
    <Pressable
      onPress={() => router.push(`/artist/${artist.id}`)}
      className="flex-row items-center gap-3 px-4 py-2 active:bg-surface-alt"
    >
      <CoverArt
        coverArt={artist.coverArt}
        url={artist.artistImageUrl}
        size={52}
        rounded="rounded-full"
        fallbackIcon="person"
      />
      <View className="flex-1">
        <Text numberOfLines={1} className="text-base font-medium text-foreground">
          {artist.name}
        </Text>
        {artist.albumCount != null ? (
          <Text className="text-sm text-muted">
            {artist.albumCount} album{artist.albumCount > 1 ? 's' : ''}
          </Text>
        ) : null}
      </View>
      <Ionicon name="chevron-forward" size={18} color={colors.muted} />
    </Pressable>
  );
});
