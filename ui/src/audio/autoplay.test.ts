import { autoplayRequest, AUTOPLAY_THRESHOLD } from './autoplay';
import { Song } from '../api/subsonic/types';

const queue = (n: number): Song[] => Array.from({ length: n }, (_, i) => ({ id: `t${i}`, title: `T${i}` }) as Song);

describe('autoplayRequest', () => {
  it('waits while enough tracks are left', () => {
    expect(autoplayRequest(queue(10), 10 - 1 - AUTOPLAY_THRESHOLD)).toBeNull();
  });

  it('seeds from the last five tracks played, excluding the whole queue', () => {
    const songs = queue(10);
    const req = autoplayRequest(songs, 8);
    expect(req?.seeds).toEqual(['t4', 't5', 't6', 't7', 't8']);
    expect(req?.exclude).toEqual(songs.map((s) => s.id));
  });

  it('skips unresolved tracks as seeds', () => {
    const songs = queue(2);
    songs[1].unresolved = true;
    expect(autoplayRequest(songs, 1)?.seeds).toEqual(['t0']);
    songs[0].unresolved = true;
    expect(autoplayRequest(songs, 1)).toBeNull();
  });

  it('ignores an empty queue', () => {
    expect(autoplayRequest([], -1)).toBeNull();
  });
});
