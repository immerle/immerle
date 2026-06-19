import { useState } from 'react';
import { Image, View } from 'react-native';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';

/**
 * A radio station logo with a graceful fallback: shows the cached cover image
 * when available, and a radio glyph when there's no logo or it fails to load.
 */
export function StationCover({ uri, size = 48, rounded = 8 }: { uri?: string; size?: number; rounded?: number }) {
  const colors = useColors();
  const [failed, setFailed] = useState(false);
  if (!uri || failed) {
    return (
      <View style={{ width: size, height: size, borderRadius: rounded }} className="items-center justify-center bg-primary/15">
        <Ionicon name="radio" size={Math.round(size * 0.5)} color={colors.primary} />
      </View>
    );
  }
  return (
    <Image
      source={{ uri }}
      style={{ width: size, height: size, borderRadius: rounded, backgroundColor: colors.surfaceAlt }}
      resizeMode="contain"
      onError={() => setFailed(true)}
    />
  );
}
