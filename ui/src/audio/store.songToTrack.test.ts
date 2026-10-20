import { ImmerleApiError } from '../api/immerle/types';
import type { ImmerleClient } from '../api/immerle/client';
import type { Song } from '../api/subsonic/types';

jest.mock('./engine', () => ({ createEngine: () => ({}) }));
jest.mock('../offline/store', () => ({ offlinePlayableUrl: async () => null }));
jest.mock('@react-native-async-storage/async-storage', () => ({
  getItem: async () => null,
  setItem: async () => {},
  removeItem: async () => {},
}));

// eslint-disable-next-line @typescript-eslint/no-require-imports
const { songToTrack } = require('./store') as typeof import('./store');

const song = { id: 'gone', title: 'Gone', artist: 'A', album: 'B', duration: 10 } as Song;
const rejectingClient = (err: Error) =>
  ({ streamUrl: () => Promise.reject(err), coverArtUrl: () => '' }) as unknown as ImmerleClient;

test('a track deleted from the catalog becomes an empty-url placeholder', async () => {
  const track = await songToTrack(rejectingClient(new ImmerleApiError(0, 'nf', 'not_found')), song, 'auto');
  expect(track.url).toBe('');
});

test('other stream-url errors still propagate', async () => {
  await expect(songToTrack(rejectingClient(new ImmerleApiError(0, 'boom', 'internal')), song, 'auto')).rejects.toThrow(
    'boom',
  );
});
