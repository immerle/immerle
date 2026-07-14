import { View } from 'react-native';
import { LinearGradient } from 'expo-linear-gradient';
import { Ionicon } from './Ionicon';

/**
 * The "Hall of Fame" artwork — a gold→amber gradient with a trophy glyph, the
 * visual counterpart to {@link LikedCover}/{@link LocalCover}. Used for the
 * pinned sidebar/tab tile (not on the Hall of Fame page itself).
 */
export function HallOfFameCover({ size, rounded = 12 }: { size: number; rounded?: number }) {
  return (
    <LinearGradient
      colors={['#fcd34d', '#b45309']}
      start={{ x: 0, y: 0 }}
      end={{ x: 1, y: 1 }}
      style={{ width: size, height: size, borderRadius: rounded }}
    >
      <View className="flex-1 items-center justify-center">
        <Ionicon name="trophy" size={Math.round(size * 0.42)} color="#ffffff" />
      </View>
    </LinearGradient>
  );
}
