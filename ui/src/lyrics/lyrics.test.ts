import { activeLineIndex, LyricLine } from './lyrics';

const lines: LyricLine[] = [
  { startMs: 0, text: 'one' },
  { startMs: 5000, text: 'two' },
  { startMs: 10000, text: 'three' },
];

describe('activeLineIndex', () => {
  it('returns -1 before the first timestamp', () => {
    expect(activeLineIndex([{ startMs: 2000, text: 'x' }], 1000)).toBe(-1);
  });

  it('picks the last line at or before the position', () => {
    expect(activeLineIndex(lines, 0)).toBe(0);
    expect(activeLineIndex(lines, 4999)).toBe(0);
    expect(activeLineIndex(lines, 5000)).toBe(1);
    expect(activeLineIndex(lines, 9999)).toBe(1);
    expect(activeLineIndex(lines, 60000)).toBe(2);
  });

  it('ignores unsynced (null) lines', () => {
    const plain: LyricLine[] = [
      { startMs: null, text: 'a' },
      { startMs: null, text: 'b' },
    ];
    expect(activeLineIndex(plain, 10000)).toBe(-1);
  });
});
