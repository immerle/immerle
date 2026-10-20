import { createEngine } from './engine.web';
import { PlayableTrack } from './types';

// Minimal HTMLAudioElement stand-in: records listeners so the test can fire
// 'ended', and tracks just enough state for the engine.
class FakeAudio {
  private listeners: Record<string, (() => void)[]> = {};
  src = '';
  currentTime = 0;
  duration = 100;
  volume = 1;
  playbackRate = 1;
  preload = '';
  playCalls = 0;
  pauseCalls = 0;
  readyState = 0;
  addEventListener(type: string, cb: () => void) {
    (this.listeners[type] ||= []).push(cb);
  }
  removeAttribute() {}
  load() {
    // Real browsers reset readyState to HAVE_NOTHING on load() and only
    // reach HAVE_METADATA asynchronously once the new source is probed.
    this.readyState = 0;
    setTimeout(() => {
      this.readyState = 1;
      this.fire('loadedmetadata');
    }, 0);
  }
  async play() {
    this.playCalls += 1;
    this.fire('playing');
  }
  pause() {
    this.pauseCalls += 1;
    this.fire('pause');
  }
  fire(type: string) {
    (this.listeners[type] ?? []).forEach((c) => c());
  }
}

let lastAudio: FakeAudio;
(global as unknown as { Audio: unknown }).Audio = function Audio() {
  lastAudio = new FakeAudio();
  return lastAudio;
};

const flush = () => new Promise((r) => setTimeout(r, 0));
const track = (id: string): PlayableTrack => ({ id, url: `http://x/${id}`, title: id });

describe('web engine setQueue', () => {
  it('loads without starting playback, so a caller can seek before playing', async () => {
    const engine = createEngine();
    await engine.setup();
    await engine.setQueue([track('a'), track('b')], 0);

    // Regression: setQueue used to call play() unconditionally, racing a
    // caller's subsequent seekTo — the seek could silently not stick on a
    // real browser (see engine.web.ts). Loading paused avoids that race.
    expect(lastAudio.playCalls).toBe(0);
    expect(engine.getState().status).not.toBe('playing');

    await engine.seekTo(42);
    expect(lastAudio.currentTime).toBe(42);
    expect(lastAudio.playCalls).toBe(0); // still hasn't started — caller decides
  });

  it('defers a seek issued right after load until metadata is ready (reload race)', async () => {
    const engine = createEngine();
    await engine.setup();
    await engine.setQueue([track('a')], 0);

    // Regression: applyRemoteQueue seeks immediately after setQueue, before
    // the source has been probed (readyState still HAVE_NOTHING). Writing
    // currentTime at that point is a "default start position" a real
    // browser is free to drop once metadata actually loads — it must not
    // be applied until then.
    const seek = engine.seekTo(30);
    expect(lastAudio.currentTime).toBe(0);

    await seek; // resolves once 'loadedmetadata' fires
    expect(lastAudio.currentTime).toBe(30);
  });
});

describe('web engine end of queue', () => {
  it('auto-advances between tracks, then rewinds to track 1 and pauses at the end', async () => {
    const engine = createEngine();
    await engine.setup();
    await engine.setQueue([track('a'), track('b')], 0);
    expect(engine.getState().index).toBe(0);

    // First track ends → advance to the second.
    lastAudio.fire('ended');
    await flush();
    expect(engine.getState().index).toBe(1);
    expect(engine.getState().status).toBe('playing');

    // Last track ends → back to track 1, paused, cursor at the start.
    lastAudio.fire('ended');
    await flush();
    expect(engine.getState().index).toBe(0);
    expect(engine.getState().status).toBe('paused');
    expect(engine.getState().position).toBe(0);
  });

  it('rewinds a single track to the start and pauses when it ends', async () => {
    const engine = createEngine();
    await engine.setup();
    await engine.setQueue([track('solo')], 0);

    lastAudio.fire('ended');
    await flush();
    expect(engine.getState().index).toBe(0);
    expect(engine.getState().status).toBe('paused');
    expect(engine.getState().position).toBe(0);
  });
});

describe('web engine media errors', () => {
  it('pauses and emits an error when the current source cannot be loaded', async () => {
    const engine = createEngine();
    const onError = jest.fn();
    await engine.setup();
    engine.on('error', onError);
    await engine.setQueue([track('missing')], 0);

    lastAudio.fire('error');

    expect(engine.getState().status).toBe('paused');
    expect(onError).toHaveBeenCalledTimes(1);
  });
});

describe('web engine replaceAt', () => {
  it('reloads the currently playing track in place and resumes playback', async () => {
    const engine = createEngine();
    await engine.setup();
    await engine.setQueue([track('a'), track('b')], 0);
    lastAudio.fire('playing');
    await flush();
    expect(engine.getState().status).toBe('playing');

    await engine.replaceAt(0, track('a-local'));
    expect(lastAudio.src).toBe('http://x/a-local');
    lastAudio.fire('playing');
    await flush();
    expect(engine.getState().status).toBe('playing');
  });

  it('swaps a non-current track without touching playback', async () => {
    const engine = createEngine();
    await engine.setup();
    await engine.setQueue([track('a'), track('b')], 0);
    const srcBeforeSwap = lastAudio.src;

    await engine.replaceAt(1, track('b-local'));
    expect(lastAudio.src).toBe(srcBeforeSwap);
  });
});

describe('web engine crossfade', () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  // Starts playing a then b with a 4s crossfade, the first element positioned
  // `left` seconds before the end of a, and fires its timeupdate.
  const playNearEnd = async (left: number, tracks = [track('a'), track('b')]) => {
    const engine = createEngine();
    await engine.setup();
    await engine.setCrossfade(4);
    await engine.setQueue(tracks, 0);
    await engine.play();
    const first = lastAudio;
    first.currentTime = first.duration - left;
    first.fire('timeupdate');
    await Promise.resolve();
    await Promise.resolve();
    return { engine, first };
  };

  it('starts the next track under the current one and swaps once the fade is done', async () => {
    const changes: number[] = [];
    const { engine, first } = await playNearEnd(3);
    engine.on('trackChange', (i) => changes.push(i));
    const second = lastAudio;

    expect(second).not.toBe(first);
    expect(second.src).toBe('http://x/b');
    expect(engine.getState().index).toBe(1);

    jest.advanceTimersByTime(2000);
    expect(second.volume).toBeGreaterThan(0.3);
    expect(second.volume).toBeLessThan(0.7);
    expect(first.volume).toBeCloseTo(1 - second.volume, 1);

    jest.advanceTimersByTime(2500);
    expect(second.volume).toBe(1);
    expect(first.pauseCalls).toBe(1);

    // The outgoing element's own end no longer moves the queue.
    first.fire('ended');
    expect(engine.getState().index).toBe(1);
    expect(changes).toEqual([]);
  });

  it('waits until the crossfade window', async () => {
    const { engine, first } = await playNearEnd(10);
    expect(lastAudio).toBe(first);
    expect(engine.getState().index).toBe(0);
  });

  it('does not fade out of the last track', async () => {
    const { first } = await playNearEnd(3, [track('solo')]);
    expect(lastAudio).toBe(first);
  });

  it('cuts the outgoing track when the user pauses mid-fade', async () => {
    const { engine, first } = await playNearEnd(3);
    const second = lastAudio;
    jest.advanceTimersByTime(1000);

    await engine.pause();
    expect(first.pauseCalls).toBe(1);
    expect(second.volume).toBe(1);
    expect(engine.getState().index).toBe(1);
  });
});
