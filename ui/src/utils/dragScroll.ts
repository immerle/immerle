import { useRef } from 'react';

/** New scrollLeft for a horizontal drag: however far scrolling was when the
 * drag started, minus however far the pointer has moved since. */
export function dragScrollLeft(startScrollLeft: number, startX: number, currentX: number): number {
  return startScrollLeft - (currentX - startX);
}

// Past this many px of movement, releasing no longer counts as a plain click
// on whatever's underneath (e.g. an album cover) — it was a scroll-drag, even
// if the release lands back on the same cover it started on.
const CLICK_THRESHOLD = 5;

type ReleaseEvent = { stopPropagation: () => void };

/**
 * ponytail: click-and-drag horizontal scrolling, web only — React Native
 * Web's ScrollView scrolls via touch/trackpad but not a mouse drag, unlike
 * native touch scrolling which already works. Spread the returned handlers
 * onto a horizontal ScrollView; native platforms never fire mouse events, so
 * this is a no-op there.
 *
 * A real drag also needs to stop the release from acting as a press on
 * whatever's under the cursor: Pressable can fire onPress straight off
 * pointerup, not just a browser click, so both are stopped in capture phase
 * — before the Pressable underneath ever sees either.
 */
export function useDragScroll() {
  const dragging = useRef(false);
  const start = useRef({ x: 0, scrollLeft: 0 });
  const wasDrag = useRef(false);

  const onRelease = (e: ReleaseEvent) => {
    dragging.current = false;
    if (wasDrag.current) e.stopPropagation();
  };

  return {
    onMouseDown: (e: { currentTarget: { scrollLeft: number }; clientX: number; preventDefault: () => void }) => {
      // Without this, mousedown starting on an <img> (album/playlist covers)
      // triggers the browser's native "drag this image out" ghost instead of
      // our scroll-drag.
      e.preventDefault();
      dragging.current = true;
      wasDrag.current = false;
      start.current = { x: e.clientX, scrollLeft: e.currentTarget.scrollLeft };
    },
    onMouseMove: (e: { currentTarget: { scrollLeft: number }; clientX: number }) => {
      if (!dragging.current) return;
      if (Math.abs(e.clientX - start.current.x) > CLICK_THRESHOLD) wasDrag.current = true;
      e.currentTarget.scrollLeft = dragScrollLeft(start.current.scrollLeft, start.current.x, e.clientX);
    },
    onMouseUpCapture: onRelease,
    onPointerUpCapture: onRelease,
    onMouseLeave: () => {
      dragging.current = false;
    },
    onClickCapture: (e: { preventDefault: () => void; stopPropagation: () => void }) => {
      if (!wasDrag.current) return;
      wasDrag.current = false;
      e.preventDefault();
      e.stopPropagation();
    },
  };
}
