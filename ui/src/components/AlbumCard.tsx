import { memo } from 'react';
import { Pressable, Text, View } from 'react-native';
import { router } from 'expo-router';
import { Album } from '../api/subsonic/types';
import { CoverArt } from './CoverArt';

/** Grid card for an album. Width is set by the parent grid column. */
export const AlbumCard = memo(function AlbumCard({ album, width }: { album: Album; width: number }) {
  return (
    <Pressable
      onPress={() => router.push(`/album/${album.id}`)}
      style={{ width }}
      className="mb-4 active:opacity-70"
    >
      <CoverArt coverArt={album.coverArt} size={width} rounded="rounded-xl" />
      <Text numberOfLines={1} className="mt-2 text-sm font-semibold text-foreground">
        {album.name}
      </Text>
      <Text numberOfLines={1} className="text-xs text-muted">
        {album.artist}
        {album.year ? ` · ${album.year}` : ''}
      </Text>
    </Pressable>
  );
});

/** Horizontal list tile used in carousels on the Home screen. */
export const AlbumTile = memo(function AlbumTile({ album, size = 140 }: { album: Album; size?: number }) {
  return (
    <Pressable
      onPress={() => router.push(`/album/${album.id}`)}
      style={{ width: size }}
      className="mr-3 active:opacity-70"
    >
      <CoverArt coverArt={album.coverArt} size={size} rounded="rounded-xl" />
      <Text numberOfLines={1} className="mt-2 text-sm font-semibold text-foreground">
        {album.name}
      </Text>
      <Text numberOfLines={1} className="text-xs text-muted">
        {album.artist}
      </Text>
      <View />
    </Pressable>
  );
});
