import { useEffect } from 'react';
import { View } from 'react-native';
import Animated, {
  Easing,
  useAnimatedStyle,
  useSharedValue,
  withDelay,
  withRepeat,
  withSequence,
  withTiming,
} from 'react-native-reanimated';
import { Ionicon } from './Ionicon';
import { palette } from '../theme/colors';

const LOGO_HEIGHT = 140;
const LOGO_WIDTH = LOGO_HEIGHT * (480 / 391);

// Notes placed around the logo in a ring, each with its own color, angle and
// bounce delay so they twinkle out of sync — a little "dancefloor" flourish.
const NOTES: { icon: string; angle: number; radius: number; size: number; color: string; delay: number }[] = [
  { icon: 'musical-notes', angle: -70, radius: 110, size: 26, color: '#1db954', delay: 0 },
  { icon: 'musical-note', angle: -10, radius: 125, size: 20, color: '#ec4899', delay: 150 },
  { icon: 'musical-note', angle: 55, radius: 105, size: 22, color: '#3b82f6', delay: 300 },
  { icon: 'musical-notes', angle: 125, radius: 120, size: 24, color: '#f59e0b', delay: 450 },
  { icon: 'musical-note', angle: 195, radius: 110, size: 20, color: '#8b5cf6', delay: 600 },
  { icon: 'musical-note', angle: 260, radius: 120, size: 22, color: '#ef4444', delay: 750 },
];

function FloatingNote({ icon, angle, radius, size, color, delay }: (typeof NOTES)[number]) {
  const bounce = useSharedValue(0);
  useEffect(() => {
    bounce.value = withDelay(
      delay,
      withRepeat(withSequence(withTiming(1, { duration: 900, easing: Easing.inOut(Easing.ease) }), withTiming(0, { duration: 900, easing: Easing.inOut(Easing.ease) })), -1),
    );
  }, [bounce, delay]);

  const rad = (angle * Math.PI) / 180;
  const x = Math.cos(rad) * radius;
  const y = Math.sin(rad) * radius;
  const style = useAnimatedStyle(() => ({
    transform: [{ translateX: x }, { translateY: y - bounce.value * 14 }, { scale: 0.85 + bounce.value * 0.3 }],
    opacity: 0.55 + bounce.value * 0.45,
  }));

  return (
    <Animated.View style={[{ position: 'absolute' }, style]}>
      <Ionicon name={icon} size={size} color={color} />
    </Animated.View>
  );
}

/** Launch screen shown while the persisted theme/session are restoring: a big
 * spinning, pulsing logo ringed by twinkling colored music notes. */
export function LaunchLoading() {
  const scale = useSharedValue(1);
  const rotate = useSharedValue(0);

  useEffect(() => {
    scale.value = withRepeat(
      withSequence(withTiming(1.12, { duration: 700, easing: Easing.inOut(Easing.ease) }), withTiming(1, { duration: 700, easing: Easing.inOut(Easing.ease) })),
      -1,
    );
    rotate.value = withRepeat(withTiming(360, { duration: 6000, easing: Easing.linear }), -1);
  }, [scale, rotate]);

  const logoStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }, { rotate: `${rotate.value}deg` }],
  }));

  return (
    <View style={{ flex: 1, backgroundColor: palette.dark.background, alignItems: 'center', justifyContent: 'center' }}>
      <View style={{ width: 1, height: 1, alignItems: 'center', justifyContent: 'center' }}>
        {NOTES.map((note, i) => (
          <FloatingNote key={i} {...note} />
        ))}
        <Animated.Image
          source={require('../../assets/logo.png')}
          style={[{ width: LOGO_WIDTH, height: LOGO_HEIGHT }, logoStyle]}
          resizeMode="contain"
        />
      </View>
    </View>
  );
}
