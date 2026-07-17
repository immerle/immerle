import { useRef } from 'react';

/** New scrollLeft for a horizontal drag: however far scrolling was when the
 * drag started, minus however far the pointer has moved since. */
export function dragScrollLeft(startScrollLeft: number, startX: number, currentX: number): number {
  return startScrollLeft - (currentX - startX);
}

/**
 * ponytail: click-and-drag horizontal scrolling, web only — React Native
 * Web's ScrollView scrolls via touch/trackpad but not a mouse drag, unlike
 * native touch scrolling which already works. Spread the returned handlers
 * onto a horizontal ScrollView; native platforms never fire mouse events, so
 * this is a no-op there.
 */
export function useDragScroll() {
  const drag = useRef<{ startX: number; startScrollLeft: number } | null>(null);
  const stop = () => {
    drag.current = null;
  };

  return {
    onMouseDown: (e: { currentTarget: { scrollLeft: number }; clientX: number; preventDefault: () => void }) => {
      // Without this, mousedown starting on an <img> (album/playlist covers)
      // triggers the browser's native "drag this image out" ghost instead of
      // our scroll-drag.
      e.preventDefault();
      drag.current = { startX: e.clientX, startScrollLeft: e.currentTarget.scrollLeft };
    },
    onMouseMove: (e: { currentTarget: { scrollLeft: number }; clientX: number }) => {
      if (!drag.current) return;
      e.currentTarget.scrollLeft = dragScrollLeft(drag.current.startScrollLeft, drag.current.startX, e.clientX);
    },
    onMouseUp: stop,
    onMouseLeave: stop,
  };
}
