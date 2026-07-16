import { View } from 'react-native';
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';

/**
 * Grip icon that arms a DraggableFlatList row for reordering. Built on the
 * Gesture API rather than a Touchable*: TouchableOpacity's internal press
 * state machine can get stuck in BEGAN when the list's outer pan gesture
 * takes over mid-press, silently no-oping the next tap. Gesture.Tap() has no
 * such carried-over state.
 *
 * Uses `onBegin` (fires on touch-down) not `onStart` (fires only once the
 * tap is recognized, near release) — the drag needs to arm immediately to
 * track the same touch's movement.
 */
export function DragHandle({
  drag,
  disabled,
  accessibilityLabel,
}: {
  drag: () => void;
  disabled?: boolean;
  accessibilityLabel: string;
}) {
  const colors = useColors();
  const gesture = Gesture.Tap()
    .enabled(!disabled)
    .onBegin(drag)
    .runOnJS(true);

  return (
    <GestureDetector gesture={gesture}>
      <View hitSlop={8} accessibilityLabel={accessibilityLabel}>
        <Ionicon name="reorder-three" size={24} color={colors.muted} />
      </View>
    </GestureDetector>
  );
}
