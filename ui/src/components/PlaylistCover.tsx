import { CoverArt } from './CoverArt';
import { PlaylistMosaic } from './PlaylistMosaic';

/**
 * A playlist's artwork: the owner-chosen custom cover when set, otherwise the
 * 2×2 mosaic of its first track covers. Used everywhere a playlist thumbnail
 * appears (detail page, sidebar, library list) so they stay consistent.
 */
export function PlaylistCover({
  coverArt,
  covers,
  size,
  rounded = 'rounded-2xl',
  fallbackIcon = 'list',
}: {
  /** Custom cover id (takes precedence over the mosaic). */
  coverArt?: string;
  /** Cover ids for the mosaic fallback. */
  covers: (string | undefined)[];
  size: number;
  rounded?: string;
  fallbackIcon?: string;
}) {
  if (coverArt) {
    return <CoverArt coverArt={coverArt} size={size} rounded={rounded} fallbackIcon={fallbackIcon} />;
  }
  return <PlaylistMosaic covers={covers} size={size} rounded={rounded} fallbackIcon={fallbackIcon} />;
}
