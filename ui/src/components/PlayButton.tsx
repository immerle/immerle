import { Pressable } from 'react-native';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';

/**
 * Spotify's signature green circular play/pause button. Black glyph on bright
 * green, with a soft shadow and a press scale. Reused on album/playlist heroes
 * and anywhere a primary "play" affordance is needed.
 */
export function PlayButton({
  playing,
  onPress,
  size = 56,
  accessibilityLabel,
  disabled,
}: {
  playing?: boolean;
  onPress?: () => void;
  size?: number;
  accessibilityLabel?: string;
  disabled?: boolean;
}) {
  const colors = useColors();
  return (
    <Pressable
      onPress={onPress}
      disabled={disabled}
      accessibilityRole="button"
      accessibilityState={{ disabled: !!disabled }}
      accessibilityLabel={accessibilityLabel ?? (playing ? 'Pause' : 'Lecture')}
      className={`items-center justify-center rounded-full bg-primary active:scale-95 ${disabled ? 'opacity-40' : ''}`}
      style={{
        width: size,
        height: size,
        shadowColor: '#000',
        shadowOpacity: 0.3,
        shadowRadius: 8,
        shadowOffset: { width: 0, height: 4 },
        elevation: 6,
      }}
    >
      <Ionicon
        name={playing ? 'pause' : 'play'}
        size={Math.round(size * 0.46)}
        color={colors.primaryForeground}
      />
    </Pressable>
  );
}
