// Regression for #271: the saved volume must reach the audio engine whichever
// of hydrateSettings() and init() finishes first (ui/app/_layout.tsx runs them
// concurrently).

const mockSetVolumeCalls: number[] = [];

jest.mock('./engine', () => ({
  createEngine: () => ({
    setup: async () => {},
    on: () => {},
    setVolume: async (v: number) => {
      mockSetVolumeCalls.push(v);
    },
  }),
}));

// The saved-volume read stays pending until the test releases it.
let releaseVolume: (v: string | null) => void = () => {};
let mockSavedVolume: Promise<string | null> = Promise.resolve(null);
jest.mock('@react-native-async-storage/async-storage', () => ({
  getItem: (key: string) => (key === 'immerle.volume.v1' ? mockSavedVolume : Promise.resolve(null)),
  setItem: async () => {},
  removeItem: async () => {},
}));

function loadStore() {
  let usePlayer!: typeof import('./store').usePlayer;
  jest.isolateModules(() => {
    // A fresh store per test: isolateModules only works with a sync require.
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    usePlayer = require('./store').usePlayer;
  });
  return usePlayer;
}

beforeEach(() => {
  mockSetVolumeCalls.length = 0;
  mockSavedVolume = new Promise((resolve) => {
    releaseVolume = resolve;
  });
  jest.useFakeTimers();
});

afterEach(() => {
  jest.clearAllTimers();
  jest.useRealTimers();
});

test('saved volume reaches an engine initialized before hydration finished', async () => {
  const usePlayer = loadStore();
  const hydrated = usePlayer.getState().hydrateSettings();
  await usePlayer.getState().init();
  releaseVolume('0.2');
  await hydrated;

  expect(usePlayer.getState().volume).toBe(0.2);
  expect(mockSetVolumeCalls[mockSetVolumeCalls.length - 1]).toBe(0.2);
});

test('saved volume is applied when hydration finishes before init', async () => {
  const usePlayer = loadStore();
  const hydrated = usePlayer.getState().hydrateSettings();
  releaseVolume('0.2');
  await hydrated;
  await usePlayer.getState().init();

  expect(mockSetVolumeCalls).toEqual([0.2]);
});
