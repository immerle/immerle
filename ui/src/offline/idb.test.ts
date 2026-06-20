import 'fake-indexeddb/auto';
import { idbPut, idbGet, idbDelete, idbHas } from './idb';

describe('offline idb', () => {
  it('puts, reads, reports presence and deletes a blob', async () => {
    const blob = new Blob(['hello'], { type: 'audio/mpeg' });

    expect(await idbHas('a.mp3')).toBe(false);
    await idbPut('a.mp3', blob);

    expect(await idbHas('a.mp3')).toBe(true);
    const got = await idbGet('a.mp3');
    expect(got).toBeInstanceOf(Blob);
    expect(got?.size).toBe(blob.size);

    await idbDelete('a.mp3');
    expect(await idbHas('a.mp3')).toBe(false);
    expect(await idbGet('a.mp3')).toBeUndefined();
  });

  it('keeps entries independent by key', async () => {
    await idbPut('x', new Blob(['x']));
    await idbPut('y', new Blob(['yy']));
    expect((await idbGet('x'))?.size).toBe(1);
    expect((await idbGet('y'))?.size).toBe(2);
    await idbDelete('x');
    expect(await idbHas('x')).toBe(false);
    expect(await idbHas('y')).toBe(true);
  });
});
