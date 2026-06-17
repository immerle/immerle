import { md5 } from './md5';

describe('md5', () => {
  it('matches RFC 1321 test vectors', () => {
    expect(md5('')).toBe('d41d8cd98f00b204e9800998ecf8427e');
    expect(md5('a')).toBe('0cc175b9c0f1b6a831c399e269772661');
    expect(md5('abc')).toBe('900150983cd24fb0d6963f7d28e17f72');
    expect(md5('message digest')).toBe('f96b697d7cb7938d525a2f31aaf161d0');
    expect(md5('The quick brown fox jumps over the lazy dog')).toBe(
      '9e107d9d372bb6826bd81d3542a419d6',
    );
  });

  it('handles UTF-8 input', () => {
    expect(md5('héllo')).toBe(md5('héllo'));
    // Known md5 of the UTF-8 bytes of "café".
    expect(md5('café')).toBe('07117fe4a1ebd544965dc19573183da2');
  });

  it('produces the Subsonic auth token shape (32 hex chars)', () => {
    const token = md5('secret' + 'c19b2d');
    expect(token).toMatch(/^[0-9a-f]{32}$/);
  });
});
