import { View } from 'react-native';
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';

/**
 * Grip icon that arms a DraggableFlatList row for reordering. Built on the
 * Gesture API rather than a Touchable* component: GenericTouchable (what
 * TouchableOpacity wraps) tracks press state in a persistent `this.STATE`
 * machine driven by its own internal LongPressGestureHandler — when the
 * list's outer pan gesture takes over the touch mid-press, that handler gets
 * cancelled rather than cleanly ended, leaving STATE stuck in BEGAN. The next
 * press then silently no-ops (the state-transition guard sees "already
 * BEGAN") until an extra throwaway tap resets it. Gesture.Tap() has no such
 * carried-over state, so every press activates the drag reliably.
 *
 * Uses `onBegin`, not `onStart`: for a tap gesture, `onStart` only fires once
 * the tap is *recognized* — which for a discrete tap resolves near release —
 * so the drag would never arm in time to track the same touch's movement.
 * `onBegin` fires immediately on touch-down, before recognition, which is
 * the actual "arm the drag now" moment we need.
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
