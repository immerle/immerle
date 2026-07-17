/** New scrollLeft for a horizontal drag: however far scrolling was when the
 * drag started, minus however far the pointer has moved since. */
export function dragScrollLeft(startScrollLeft: number, startX: number, currentX: number): number {
  return startScrollLeft - (currentX - startX);
}
