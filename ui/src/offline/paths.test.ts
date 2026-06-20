import { offlineFileName } from './paths';

describe('offlineFileName', () => {
  it('combines id and lowercased suffix', () => {
    expect(offlineFileName('abc-123', 'mp3')).toBe('abc-123.mp3');
    expect(offlineFileName('abc', 'OPUS')).toBe('abc.opus');
  });

  it('defaults the extension when the suffix is missing or empty', () => {
    expect(offlineFileName('abc')).toBe('abc.audio');
    expect(offlineFileName('abc', '')).toBe('abc.audio');
    expect(offlineFileName('abc', '!!')).toBe('abc.audio');
  });

  it('sanitizes unsafe characters into a single safe segment', () => {
    expect(offlineFileName('a/b:c', 'm4a')).toBe('a_b_c.m4a');
    expect(offlineFileName('id', 'm p3')).toBe('id.mp3');
    expect(offlineFileName('uuid-1234_5', 'flac')).toBe('uuid-1234_5.flac');
  });
});
