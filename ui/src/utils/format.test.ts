import { formatBytes, formatCount, formatDuration } from './format';

describe('formatDuration', () => {
  it('formats seconds as m:ss and h:mm:ss', () => {
    expect(formatDuration(0)).toBe('0:00');
    expect(formatDuration(5)).toBe('0:05');
    expect(formatDuration(65)).toBe('1:05');
    expect(formatDuration(3661)).toBe('1:01:01');
  });

  it('is defensive against bad input', () => {
    expect(formatDuration(undefined)).toBe('0:00');
    expect(formatDuration(-10)).toBe('0:00');
    expect(formatDuration(NaN)).toBe('0:00');
  });
});

describe('formatBytes', () => {
  it('scales to human units', () => {
    expect(formatBytes(0)).toBe('0 o');
    expect(formatBytes(512)).toBe('512 o');
    expect(formatBytes(1024)).toBe('1.0 Ko');
    expect(formatBytes(1024 * 1024 * 1.5)).toBe('1.5 Mo');
  });
});

describe('formatCount', () => {
  it('abbreviates large numbers', () => {
    expect(formatCount(0)).toBe('0');
    expect(formatCount(999)).toBe('999');
    expect(formatCount(12_345)).toBe('12.3k');
    expect(formatCount(1_500_000)).toBe('1.5M');
  });
});
