import { useRef, useState } from 'react';
import { ScrollView, ScrollViewProps, StyleSheet, View } from 'react-native';
import { dragScrollLeft } from '../utils/dragScroll';

// Past this many px of movement, releasing no longer counts as a plain click
// on whatever's underneath (e.g. an album cover) — it was a scroll-drag, even
// if the release lands back on the same cover it started on.
const CLICK_THRESHOLD = 5;

type MouseishEvent = { currentTarget: { scrollLeft: number }; clientX: number; preventDefault: () => void };

/**
 * ponytail: horizontal ScrollView with click-and-drag scrolling, web only —
 * React Native Web's ScrollView already scrolls via touch/trackpad but not a
 * mouse drag, and native platforms never fire mouse events so this is a
 * no-op there.
 *
 * A drag past CLICK_THRESHOLD also raises a transparent overlay on top of
 * the row for as long as the browser's trailing click could still land —
 * otherwise releasing back on the cover you started dragging on opens/plays
 * it. This can't be done by intercepting the click event itself: RN Web's
 * View only forwards a fixed prop whitelist to the DOM (see
 * modules/forwardedProps) which excludes onClickCapture and any Capture-phase
 * mouse/pointer variant, so a bubble-phase handler on an ancestor always
 * runs after the Pressable's own — too late to stop it.
 */
export function DragScrollRow({ children, ...props }: ScrollViewProps) {
  const dragging = useRef(false);
  const start = useRef({ x: 0, scrollLeft: 0 });
  const [blocking, setBlocking] = useState(false);

  const onMouseDown = (e: MouseishEvent) => {
    // Without this, mousedown starting on an <img> (album/playlist covers)
    // triggers the browser's native "drag this image out" ghost instead of
    // our scroll-drag.
    e.preventDefault();
    dragging.current = true;
    start.current = { x: e.clientX, scrollLeft: e.currentTarget.scrollLeft };
  };
  const onMouseMove = (e: MouseishEvent) => {
    if (!dragging.current) return;
    if (!blocking && Math.abs(e.clientX - start.current.x) > CLICK_THRESHOLD) setBlocking(true);
    e.currentTarget.scrollLeft = dragScrollLeft(start.current.scrollLeft, start.current.x, e.clientX);
  };
  const stop = () => {
    dragging.current = false;
    // The click for this gesture (if any) fires synchronously right after
    // mouseup, before this timeout runs — the overlay is still up for it.
    if (blocking) setTimeout(() => setBlocking(false), 0);
  };
  // react-native's ScrollViewProps doesn't declare web mouse events; spread as
  // a variable (rather than literal JSX attributes) to sidestep the excess
  // property check — react-native-web still forwards them to the DOM.
  const mouseHandlers = { onMouseDown, onMouseMove, onMouseUp: stop, onMouseLeave: stop };

  return (
    <View style={styles.wrapper}>
      <ScrollView horizontal {...props} {...mouseHandlers}>
        {children}
      </ScrollView>
      {blocking ? <View pointerEvents="auto" style={StyleSheet.absoluteFill} /> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrapper: { position: 'relative' },
});
