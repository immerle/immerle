import { Image } from 'expo-image';
import { View } from 'react-native';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';
import { useAuth } from '../auth/store';

interface CoverArtProps {
  /** Subsonic coverArt id. */
  coverArt?: string;
  /** Pre-built URL (overrides coverArt). */
  url?: string;
  size: number;
  /** Tailwind rounding class. */
  rounded?: string;
  /** Icon shown when there is no artwork. */
  fallbackIcon?: string;
}

const blurhashPlaceholder = 'L6PZfSi_.AyE_3t7t7R**0o#DgR4';

/**
 * Cached cover-art image. expo-image keeps a disk + memory cache keyed by URL,
 * so re-rendering large lists doesn't refetch. Builds the authenticated URL
 * from a coverArt id via the live client when `url` isn't supplied.
 */
export function CoverArt({ coverArt, url, size, rounded = 'rounded-lg', fallbackIcon = 'musical-notes' }: CoverArtProps) {
  const colors = useColors();
  const resolved = useCoverUrl(coverArt, size);
  const source = url ?? resolved;

  if (!source) {
    return (
      <View
        className={`items-center justify-center bg-surface-alt ${rounded}`}
        style={{ width: size, height: size }}
      >
        <Ionicon name={fallbackIcon} size={Math.round(size * 0.4)} color={colors.muted} />
      </View>
    );
  }

  return (
    <Image
      source={{ uri: source }}
      style={{ width: size, height: size }}
      className={rounded}
      contentFit="cover"
      transition={200}
      placeholder={{ blurhash: blurhashPlaceholder }}
      cachePolicy="memory-disk"
    />
  );
}

// Resolve a coverArt id to a URL using the live client.
function useCoverUrl(coverArt: string | undefined, size: number): string | undefined {
  const client = useAuth((s) => s.client);
  if (!coverArt || !client) return undefined;
  return client.coverArtUrl(coverArt, size);
}
