import { useRef } from 'react';

/** New scrollLeft for a horizontal drag: however far scrolling was when the
 * drag started, minus however far the pointer has moved since. */
export function dragScrollLeft(startScrollLeft: number, startX: number, currentX: number): number {
  return startScrollLeft - (currentX - startX);
}

// Past this many px of movement, mouseup no longer counts as a plain click on
// whatever's underneath (e.g. an album cover) — it was a scroll-drag.
const CLICK_THRESHOLD = 5;

/**
 * ponytail: click-and-drag horizontal scrolling, web only — React Native
 * Web's ScrollView scrolls via touch/trackpad but not a mouse drag, unlike
 * native touch scrolling which already works. Spread the returned handlers
 * onto a horizontal ScrollView; native platforms never fire mouse events, so
 * this is a no-op there.
 */
export function useDragScroll() {
  const drag = useRef<{ startX: number; startScrollLeft: number; moved: boolean } | null>(null);
  const suppressClick = useRef(false);
  const stop = () => {
    if (drag.current?.moved) suppressClick.current = true;
    drag.current = null;
  };

  return {
    onMouseDown: (e: { currentTarget: { scrollLeft: number }; clientX: number; preventDefault: () => void }) => {
      // Without this, mousedown starting on an <img> (album/playlist covers)
      // triggers the browser's native "drag this image out" ghost instead of
      // our scroll-drag.
      e.preventDefault();
      drag.current = { startX: e.clientX, startScrollLeft: e.currentTarget.scrollLeft, moved: false };
    },
    onMouseMove: (e: { currentTarget: { scrollLeft: number }; clientX: number }) => {
      if (!drag.current) return;
      if (Math.abs(e.clientX - drag.current.startX) > CLICK_THRESHOLD) drag.current.moved = true;
      e.currentTarget.scrollLeft = dragScrollLeft(drag.current.startScrollLeft, drag.current.startX, e.clientX);
    },
    onMouseUp: stop,
    onMouseLeave: stop,
    // Dragging past the threshold ends in a mouseup on top of whatever's
    // under the cursor (e.g. an album tile), which the browser follows with
    // a click there — swallow just that one click so it doesn't open/play
    // the thing you were only trying to scroll past.
    onClickCapture: (e: { preventDefault: () => void; stopPropagation: () => void }) => {
      if (!suppressClick.current) return;
      suppressClick.current = false;
      e.preventDefault();
      e.stopPropagation();
    },
  };
}
