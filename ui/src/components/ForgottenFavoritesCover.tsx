import { View } from 'react-native';
import { LinearGradient } from 'expo-linear-gradient';
import { Ionicon } from './Ionicon';

/**
 * The "Favoris oubliés" artwork — a slate→indigo gradient with a time glyph
 * (favorites left behind), the visual counterpart to {@link LikedCover}.
 */
export function ForgottenFavoritesCover({ size, rounded = 12 }: { size: number; rounded?: number }) {
  return (
    <LinearGradient
      colors={['#94a3b8', '#312e81']}
      start={{ x: 0, y: 0 }}
      end={{ x: 1, y: 1 }}
      style={{ width: size, height: size, borderRadius: rounded }}
    >
      <View className="flex-1 items-center justify-center">
        <Ionicon name="time-outline" size={Math.round(size * 0.42)} color="#ffffff" />
      </View>
    </LinearGradient>
  );
}
