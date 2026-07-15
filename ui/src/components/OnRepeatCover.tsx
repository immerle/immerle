import { View } from 'react-native';
import { LinearGradient } from 'expo-linear-gradient';
import { Ionicon } from './Ionicon';

/**
 * The "On Repeat" artwork — a violet→blue gradient with a repeat glyph, the
 * visual counterpart to {@link LikedCover}/{@link TopMonthCover}.
 */
export function OnRepeatCover({ size, rounded = 12 }: { size: number; rounded?: number }) {
  return (
    <LinearGradient
      colors={['#818cf8', '#1e3a8a']}
      start={{ x: 0, y: 0 }}
      end={{ x: 1, y: 1 }}
      style={{ width: size, height: size, borderRadius: rounded }}
    >
      <View className="flex-1 items-center justify-center">
        <Ionicon name="repeat" size={Math.round(size * 0.42)} color="#ffffff" />
      </View>
    </LinearGradient>
  );
}
