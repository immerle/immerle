import { View } from 'react-native';
import { LinearGradient } from 'expo-linear-gradient';
import { Ionicon } from './Ionicon';

/**
 * The "Musiques locales" artwork — a teal→green gradient with an upload glyph,
 * the visual counterpart to {@link LikedCover}. Used for the virtual local
 * playlist tile and its detail hero.
 */
export function LocalCover({ size, rounded = 12 }: { size: number; rounded?: number }) {
  return (
    <LinearGradient
      colors={['#2dd4bf', '#0e9f6e']}
      start={{ x: 0, y: 0 }}
      end={{ x: 1, y: 1 }}
      style={{ width: size, height: size, borderRadius: rounded }}
    >
      <View className="flex-1 items-center justify-center">
        <Ionicon name="cloud-upload" size={Math.round(size * 0.42)} color="#ffffff" />
      </View>
    </LinearGradient>
  );
}
