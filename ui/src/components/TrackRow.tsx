import { memo } from 'react';
import { Pressable, Text, View } from 'react-native';
import { Song } from '../api/subsonic/types';
import { CommentQuote } from './CommentQuote';
import { CoverArt } from './CoverArt';
import { Ionicon } from './Ionicon';
import { IconButton } from './ui';
import { useDownloads } from '../offline/store';
import { formatDuration } from '../utils/format';
import { useColors } from '../theme/colors';

// Medal color per rank (1/2/3); ranks below 3 fall back to the muted theme color.
const RANK_COLOR: Record<number, string> = { 1: '#f2c94c', 2: '#c0c0c0', 3: '#cd7f32' };

interface TrackRowProps {
  song: Song;
  /** Highlight when this row is the active track. */
  active?: boolean;
  showArtwork?: boolean;
  /** Track number override (defaults to song.track). */
  number?: number;
  /** 1-based rank shown as a colored badge to the left of the cover (used by
   * Hall of Fame) — independent of showArtwork/number, so both can render. */
  rank?: number;
  /** Prefixed to the artist subtitle (e.g. "Song" in a mixed-type search result list). */
  typeLabel?: string;
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
  rank,
  typeLabel,
  onPress,
  onMore,
}: TrackRowProps) {
  const colors = useColors();
  const downloaded = useDownloads((s) => !song.unresolved && !!s.entries[song.id]);
  return (
    <Pressable
      onPress={onPress}
      onLongPress={onMore}
      className="flex-row items-center gap-3 px-4 py-2 active:bg-surface-alt"
    >
      {rank ? (
        <View className="w-6 items-center">
          <Text className="text-sm font-bold" style={{ color: RANK_COLOR[rank] ?? colors.muted }}>
            {rank}
          </Text>
        </View>
      ) : null}
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
          {typeLabel ? `${typeLabel} · ` : ''}
          {song.artist ?? 'Artiste inconnu'}
        </Text>
        {song.comment ? <CommentQuote comment={song.comment} className="text-xs italic text-muted" /> : null}
      </View>
      {song.unresolved ? (
        <Ionicon name="help-circle-outline" size={15} color={colors.muted} />
      ) : downloaded ? (
        <Ionicon name="cloud-done" size={15} color={colors.muted} />
      ) : null}
      <Text className="text-xs text-muted">{song.unresolved ? '' : formatDuration(song.duration)}</Text>
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
