import { dragScrollLeft } from './dragScroll';

describe('dragScrollLeft', () => {
  it('scrolls opposite the drag direction, relative to the starting offset', () => {
    expect(dragScrollLeft(100, 50, 30)).toBe(120); // dragged left 20px -> content scrolls right 20px
    expect(dragScrollLeft(100, 50, 70)).toBe(80); // dragged right 20px -> content scrolls left 20px
    expect(dragScrollLeft(0, 0, 0)).toBe(0); // no movement -> no change
  });
});
