import { View } from 'react-native';
import { LinearGradient } from 'expo-linear-gradient';
import { Ionicon } from './Ionicon';

/**
 * The "Top de votre mois" artwork — an orange→red gradient with a trending-up
 * glyph, the visual counterpart to {@link LikedCover}/{@link HallOfFameCover}.
 */
export function TopMonthCover({ size, rounded = 12 }: { size: number; rounded?: number }) {
  return (
    <LinearGradient
      colors={['#fb923c', '#b91c1c']}
      start={{ x: 0, y: 0 }}
      end={{ x: 1, y: 1 }}
      style={{ width: size, height: size, borderRadius: rounded }}
    >
      <View className="flex-1 items-center justify-center">
        <Ionicon name="trending-up" size={Math.round(size * 0.42)} color="#ffffff" />
      </View>
    </LinearGradient>
  );
}
