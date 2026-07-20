import { useCallback } from 'react';
import { FlashList } from '@shopify/flash-list';
import { Song } from '../api/subsonic/types';
import { TrackRow } from './TrackRow';
import { usePlayer } from '../audio/store';
import { useTrackMenu } from './trackMenu';

interface TrackListProps {
  songs: Song[];
  showArtwork?: boolean;
  /** Header rendered above the list (e.g. album hero). */
  header?: React.ReactElement;
  /** Footer (e.g. bottom padding for the mini-player). */
  footer?: React.ReactElement;
  refreshing?: boolean;
  onRefresh?: () => void;
  /** Shows each row's 1-based position as a colored rank badge to the left of
   * the cover (medal colors for 1-3) — used by the Hall of Fame list. */
  showRank?: boolean;
  /** When set, each row's menu offers an "Edit" action (used by the local library). */
  onEditTrack?: (song: Song) => void;
  /** Called instead of the normal play-from-here behavior when the tapped row
   * is unresolved (a federated-playlist track pending resolution — see
   * `Song.unresolved`). Required for lists that may contain such rows. */
  onPlayUnresolved?: (song: Song, index: number) => void;
  /** Playlist these songs belong to, if any — passed through to playSongs so an
   * unresolved track hit later in playback can resolve itself instead of just
   * skipping (see resolveAndPlayUnresolved in audio/store.ts). */
  playlistId?: string;
}

/**
 * Virtualized track list. Built on FlashList so libraries with tens of
 * thousands of songs scroll smoothly (only visible rows are mounted). Tapping a
 * row plays the entire list starting at that index; the active row is
 * highlighted from the player store.
 */
export function TrackList({
  songs,
  showArtwork = true,
  header,
  footer,
  refreshing,
  onRefresh,
  showRank = false,
  onEditTrack,
  onPlayUnresolved,
  playlistId,
}: TrackListProps) {
  const playSongs = usePlayer((s) => s.playSongs);
  const current = usePlayer((s) => (s.index >= 0 ? s.songs[s.index]?.id : undefined));
  const openMenu = useTrackMenu((s) => s.open);

  const renderItem = useCallback(
    ({ item, index }: { item: Song; index: number }) => (
      <TrackRow
        song={item}
        active={item.id === current}
        showArtwork={showArtwork}
        rank={showRank ? index + 1 : undefined}
        onPress={() => (item.unresolved ? onPlayUnresolved?.(item, index) : playSongs(songs, index, playlistId))}
        onMore={() => openMenu(item, onEditTrack ? { onEdit: onEditTrack } : undefined)}
      />
    ),
    [songs, current, showArtwork, showRank, playSongs, openMenu, onEditTrack, onPlayUnresolved, playlistId],
  );

  return (
    <FlashList
      data={songs}
      keyExtractor={(item, i) => `${item.id}:${i}`}
      renderItem={renderItem}
      estimatedItemSize={64}
      ListHeaderComponent={header}
      ListFooterComponent={footer}
      refreshing={refreshing}
      onRefresh={onRefresh}
      contentContainerStyle={{ paddingBottom: 8 }}
    />
  );
}
