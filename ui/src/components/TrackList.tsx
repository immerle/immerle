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
  /** When set, each row's menu offers an "Edit" action (used by the local library). */
  onEditTrack?: (song: Song) => void;
  /** Called instead of the normal play-from-here behavior when the tapped row
   * is unresolved (a federated-playlist track pending resolution — see
   * `Song.unresolved`). Required for lists that may contain such rows. */
  onPlayUnresolved?: (song: Song, index: number) => void;
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
  onEditTrack,
  onPlayUnresolved,
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
        number={index + 1}
        onPress={() => (item.unresolved ? onPlayUnresolved?.(item, index) : playSongs(songs, index))}
        onMore={() => openMenu(item, onEditTrack ? { onEdit: onEditTrack } : undefined)}
      />
    ),
    [songs, current, showArtwork, playSongs, openMenu, onEditTrack, onPlayUnresolved],
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
