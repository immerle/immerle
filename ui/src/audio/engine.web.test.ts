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
  addEventListener(type: string, cb: () => void) {
    (this.listeners[type] ||= []).push(cb);
  }
  removeAttribute() {}
  load() {}
  async play() {
    this.playCalls += 1;
    this.fire('playing');
  }
  pause() {
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
    // caller's subsequent seekTo — a fresh source could start audibly
    // playing from 0 before the seek landed, and on a real browser that
    // early seek could silently not stick (see engine.web.ts). Loading
    // paused means a caller's seekTo below is never in that race.
    expect(lastAudio.playCalls).toBe(0);
    expect(engine.getState().status).not.toBe('playing');

    await engine.seekTo(42);
    expect(lastAudio.currentTime).toBe(42);
    expect(lastAudio.playCalls).toBe(0); // still hasn't started — caller decides
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
