import { View } from 'react-native';
import { CoverArt } from './CoverArt';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';

/**
 * Playlist artwork as a 2×2 mosaic of the first track covers (up to 4):
 * - 0 covers → a tinted placeholder with an icon
 * - 1 cover  → that single cover
 * - 2–4      → a 2×2 grid (fewer than 4 distinct covers are cycled to fill it)
 */
export function PlaylistMosaic({
  covers,
  size,
  rounded = 'rounded-2xl',
  fallbackIcon = 'list',
}: {
  covers: (string | undefined)[];
  size: number;
  rounded?: string;
  fallbackIcon?: string;
}) {
  const colors = useColors();
  const ids = covers.filter((c): c is string => !!c);

  if (ids.length === 0) {
    return (
      <View className={`items-center justify-center bg-surface-alt ${rounded}`} style={{ width: size, height: size }}>
        <Ionicon name={fallbackIcon} size={Math.round(size * 0.4)} color={colors.muted} />
      </View>
    );
  }
  if (ids.length === 1) {
    return <CoverArt coverArt={ids[0]} size={size} rounded={rounded} fallbackIcon={fallbackIcon} />;
  }

  // 4+ → first 4; 2 → checkerboard tiling (c0,c1,c1,c0); 3 → cycle to fill.
  const cells =
    ids.length >= 4
      ? ids.slice(0, 4)
      : ids.length === 2
        ? [ids[0], ids[1], ids[1], ids[0]]
        : Array.from({ length: 4 }, (_, i) => ids[i % ids.length]);
  const half = Math.floor(size / 2);
  return (
    <View
      className={`overflow-hidden ${rounded}`}
      style={{ width: half * 2, height: half * 2, flexDirection: 'row', flexWrap: 'wrap' }}
    >
      {cells.map((c, i) => (
        <CoverArt key={i} coverArt={c} size={half} rounded="rounded-none" />
      ))}
    </View>
  );
}
