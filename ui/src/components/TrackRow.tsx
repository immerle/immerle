import { memo } from 'react';
import { Pressable, Text, View } from 'react-native';
import { Song } from '../api/subsonic/types';
import { CoverArt } from './CoverArt';
import { IconButton } from './ui';
import { formatDuration } from '../utils/format';
import { useColors } from '../theme/colors';

interface TrackRowProps {
  song: Song;
  /** Highlight when this row is the active track. */
  active?: boolean;
  showArtwork?: boolean;
  /** Track number override (defaults to song.track). */
  number?: number;
  onPress: () => void;
  onMore?: () => void;
}

/**
 * A single track row. Memoized because these render in long virtualized lists;
 * the parent passes stable callbacks so re-renders stay cheap during scroll.
 */
export const TrackRow = memo(function TrackRow({
  song,
  active,
  showArtwork = true,
  number,
  onPress,
  onMore,
}: TrackRowProps) {
  const colors = useColors();
  return (
    <Pressable
      onPress={onPress}
      onLongPress={onMore}
      className="flex-row items-center gap-3 px-4 py-2 active:bg-surface-alt"
    >
      {showArtwork ? (
        <CoverArt coverArt={song.coverArt} size={48} rounded="rounded-md" />
      ) : (
        <View className="w-7 items-center">
          <Text className={`text-sm ${active ? 'text-primary' : 'text-muted'}`}>
            {number ?? song.track ?? '•'}
          </Text>
        </View>
      )}
      <View className="flex-1">
        <Text
          numberOfLines={1}
          className={`text-base ${active ? 'font-semibold text-primary' : 'text-foreground'}`}
        >
          {song.title}
        </Text>
        <Text numberOfLines={1} className="text-sm text-muted">
          {song.artist ?? 'Artiste inconnu'}
        </Text>
      </View>
      <Text className="text-xs text-muted">{formatDuration(song.duration)}</Text>
      {onMore ? (
        <IconButton
          name="ellipsis-horizontal"
          size={20}
          color={colors.muted}
          onPress={onMore}
          accessibilityLabel="Plus d'options"
        />
      ) : null}
    </Pressable>
  );
});
