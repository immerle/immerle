import { useEffect } from 'react';
import { View } from 'react-native';
import Animated, { Easing, useAnimatedStyle, useSharedValue, withRepeat, withSequence, withTiming } from 'react-native-reanimated';
import { palette } from '../theme/colors';

const LOGO_HEIGHT = 80;
const LOGO_WIDTH = LOGO_HEIGHT * (480 / 391);

/** Launch screen shown while the persisted theme/session are restoring: a
 * slowly pulsing logo. */
export function LaunchLoading() {
  const scale = useSharedValue(1);

  useEffect(() => {
    scale.value = withRepeat(
      withSequence(withTiming(1.12, { duration: 1400, easing: Easing.inOut(Easing.ease) }), withTiming(1, { duration: 1400, easing: Easing.inOut(Easing.ease) })),
      -1,
    );
  }, [scale]);

  const logoStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
  }));

  return (
    <View style={{ flex: 1, backgroundColor: palette.dark.background, alignItems: 'center', justifyContent: 'center' }}>
      <Animated.Image
        source={require('../../assets/logo.png')}
        style={[{ width: LOGO_WIDTH, height: LOGO_HEIGHT }, logoStyle]}
        resizeMode="contain"
      />
    </View>
  );
}
