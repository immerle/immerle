import { EngineEmitter } from './emitter';
import { AudioEngine, EngineState, PlayableTrack, RepeatMode } from './types';

// HTMLMediaElement.readyState value for "has metadata" (HAVE_METADATA). Used
// as a literal rather than the DOM constant so this stays testable outside a
// real browser.
const HAVE_METADATA = 1;
// Volume ramp step during a crossfade.
const FADE_TICK_MS = 50;

/**
 * Web audio engine: a single `HTMLAudioElement` plus the MediaSession API for
 * OS-level transport controls (play/pause/next/prev and now-playing metadata on
 * the lockscreen / media keys / notification shade).
 *
 * The queue is managed in JS; the current element only ever holds the current
 * track. With a crossfade set, a second element is created on the first
 * transition: the next track starts on it while the current one fades out,
 * then the two swap roles. Gapless playback is not attempted.
 */
class WebAudioEngine implements AudioEngine {
  private audio: HTMLAudioElement | null = null;
  private queue: PlayableTrack[] = [];
  private index = -1;
  private repeat: RepeatMode = 'off';
  private emitter = new EngineEmitter();
  private status: EngineState['status'] = 'idle';
  private volume = 1;
  private crossfade = 0;
  /** The element fading out during a crossfade, and its volume ramp. */
  private fade: { outgoing: HTMLAudioElement; timer: ReturnType<typeof setInterval> | null } | null = null;
  /** The idle element left over from the last crossfade, reused for the next. */
  private spare: HTMLAudioElement | null = null;

  async setup(): Promise<void> {
    if (this.audio) return;
    this.audio = this.createElement();
    this.wireMediaSession();
  }

  /** Builds an audio element whose events only count while it is the current one. */
  private createElement(): HTMLAudioElement {
    const audio = new Audio();
    audio.preload = 'auto';
    const on = (type: string, handler: () => void) =>
      audio.addEventListener(type, () => {
        if (audio === this.audio) handler();
      });
    on('playing', () => this.setStatus('playing'));
    on('pause', () => {
      if (this.status !== 'ended') this.setStatus('paused');
    });
    on('waiting', () => this.setStatus('loading'));
    on('error', () => {
      this.setStatus('paused');
      this.emitter.emit('error');
    });
    on('ended', () => this.handleEnded());
    on('timeupdate', () => {
      this.emitter.emit('progress', audio.currentTime, audio.duration || 0);
      this.updatePositionState();
      this.maybeStartCrossfade();
    });
    on('loadedmetadata', () => this.emitState());
    return audio;
  }

  private get el(): HTMLAudioElement {
    if (!this.audio) throw new Error('Web audio engine not set up');
    return this.audio;
  }

  async setQueue(tracks: PlayableTrack[], startIndex: number): Promise<void> {
    this.queue = [...tracks];
    this.index = tracks.length ? Math.max(0, Math.min(startIndex, tracks.length - 1)) : -1;
    if (this.index >= 0) {
      await this.load(this.index);
    }
  }

  async add(tracks: PlayableTrack[]): Promise<void> {
    this.queue.push(...tracks);
    if (this.index === -1 && this.queue.length) {
      this.index = 0;
      await this.load(0);
    }
  }

  async removeAt(index: number): Promise<void> {
    if (index < 0 || index >= this.queue.length) return;
    this.queue.splice(index, 1);
    if (index < this.index) this.index -= 1;
    else if (index === this.index) {
      if (this.index >= this.queue.length) this.index = this.queue.length - 1;
      if (this.index >= 0) await this.load(this.index);
      else this.stop();
    }
    this.emitState();
  }

  async replaceAt(index: number, track: PlayableTrack): Promise<void> {
    if (index < 0 || index >= this.queue.length) return;
    this.queue[index] = track;
    if (index !== this.index) return;
    const wasPlaying = this.status === 'playing';
    await this.load(index);
    if (wasPlaying) await this.play();
  }

  async move(from: number, to: number): Promise<void> {
    if (from === to) return;
    const [item] = this.queue.splice(from, 1);
    if (!item) return;
    this.queue.splice(to, 0, item);
    // Track the currently playing item across the move.
    if (from === this.index) this.index = to;
    else if (from < this.index && to >= this.index) this.index -= 1;
    else if (from > this.index && to <= this.index) this.index += 1;
    this.emitState();
  }

  private async load(index: number): Promise<void> {
    const track = this.queue[index];
    if (!track) return;
    this.endFade();
    this.setStatus('loading');
    this.el.src = track.url;
    this.el.load();
    this.updateMediaMetadata(track);
    this.emitState();
  }

  async play(): Promise<void> {
    if (this.index === -1) return;
    await this.el.play().catch(() => undefined);
  }

  async pause(): Promise<void> {
    this.endFade();
    this.el.pause();
  }

  async skipTo(index: number): Promise<void> {
    if (index < 0 || index >= this.queue.length) return;
    this.index = index;
    await this.load(index);
    await this.play();
    this.emitter.emit('trackChange', index);
  }

  async next(): Promise<void> {
    if (this.repeat === 'track') return this.skipTo(this.index);
    const nextIndex = this.index + 1;
    if (nextIndex < this.queue.length) return this.skipTo(nextIndex);
    if (this.repeat === 'queue' && this.queue.length) return this.skipTo(0);
    return this.resetToStart();
  }

  // End of queue: rewind to track 1 and pause instead of tearing down.
  private async resetToStart(): Promise<void> {
    if (!this.queue.length) return this.stop();
    this.index = 0;
    await this.load(0);
    this.setStatus('paused');
    this.emitter.emit('trackChange', 0);
  }

  async previous(): Promise<void> {
    // Standard behavior: restart current track if >3s in, else go back.
    if (this.el.currentTime > 3) {
      this.el.currentTime = 0;
      return;
    }
    const prevIndex = this.index - 1;
    if (prevIndex >= 0) return this.skipTo(prevIndex);
    if (this.repeat === 'queue' && this.queue.length) return this.skipTo(this.queue.length - 1);
    this.el.currentTime = 0;
  }

  async seekTo(seconds: number): Promise<void> {
    this.endFade();
    // Setting currentTime while readyState is HAVE_NOTHING (e.g. right after
    // load() on a freshly-set src) is only a "default start position" per
    // spec — browsers are free to drop it once real metadata arrives. Wait
    // for metadata first so a seek issued right after setQueue() actually
    // sticks instead of silently playing from 0.
    if (this.el.readyState < HAVE_METADATA) {
      await new Promise<void>((resolve) => {
        this.el.addEventListener('loadedmetadata', () => resolve(), { once: true });
      });
    }
    this.el.currentTime = seconds;
    this.updatePositionState();
  }

  async setRepeatMode(mode: RepeatMode): Promise<void> {
    this.repeat = mode;
  }

  async setVolume(volume: number): Promise<void> {
    this.volume = Math.max(0, Math.min(1, volume));
    if (!this.fade) this.el.volume = this.volume; // a running fade ramps toward it
  }

  async setCrossfade(seconds: number): Promise<void> {
    this.crossfade = Math.max(0, seconds);
  }

  getState(): EngineState {
    return {
      status: this.status,
      index: this.index,
      position: this.audio?.currentTime ?? 0,
      duration: this.audio?.duration || this.queue[this.index]?.duration || 0,
    };
  }

  on: AudioEngine['on'] = (event, handler) => this.emitter.on(event, handler);

  async destroy(): Promise<void> {
    this.endFade();
    if (this.audio) {
      this.audio.pause();
      this.audio.src = '';
      this.audio = null;
    }
    this.queue = [];
    this.index = -1;
    this.setStatus('idle');
  }

  // --- internals -----------------------------------------------------------

  /** The track a natural end moves to, or -1 when playback stops there. */
  private nextIndex(): number {
    if (this.repeat === 'track') return -1;
    if (this.index + 1 < this.queue.length) return this.index + 1;
    return this.repeat === 'queue' && this.queue.length > 1 ? 0 : -1;
  }

  /**
   * Starts blending into the next track once the current one is within the
   * crossfade window. A progressively transcoded stream reports no duration
   * until it's done, so the track's known duration stands in for it. Tracks
   * shorter than twice the window play out normally.
   */
  private maybeStartCrossfade(): void {
    if (!this.crossfade || this.fade || this.status !== 'playing') return;
    const next = this.nextIndex();
    if (next === -1) return;
    const duration =
      Number.isFinite(this.el.duration) && this.el.duration > 0 ? this.el.duration : this.queue[this.index]?.duration ?? 0;
    if (duration < this.crossfade * 2 || duration - this.el.currentTime > this.crossfade) return;
    void this.startCrossfade(next);
  }

  private async startCrossfade(next: number): Promise<void> {
    const track = this.queue[next];
    if (!track) return;
    const outgoing = this.el;
    const incoming = this.spare ?? this.createElement();
    this.spare = null;
    this.fade = { outgoing, timer: null };
    // Swap first: from here on, events (and controls) follow the next track.
    this.audio = incoming;
    this.index = next;
    incoming.volume = 0;
    incoming.src = track.url;
    incoming.load();
    this.updateMediaMetadata(track);
    this.emitter.emit('trackChange', next);
    this.emitState();
    await incoming.play().catch(() => undefined);
    if (this.fade?.outgoing !== outgoing) return; // cancelled meanwhile

    const start = Date.now();
    const ms = this.crossfade * 1000;
    this.fade.timer = setInterval(() => {
      const k = Math.min(1, (Date.now() - start) / ms);
      incoming.volume = this.volume * k;
      outgoing.volume = this.volume * (1 - k);
      if (k >= 1) this.endFade();
    }, FADE_TICK_MS);
  }

  /** Stops a running crossfade: the outgoing track is cut, the current one at full volume. */
  private endFade(): void {
    if (!this.fade) return;
    const { outgoing, timer } = this.fade;
    this.fade = null;
    if (timer) clearInterval(timer);
    outgoing.pause();
    outgoing.removeAttribute('src');
    this.spare = outgoing;
    if (this.audio) this.audio.volume = this.volume;
  }

  private handleEnded(): void {
    this.setStatus('ended');
    void this.next();
  }

  private stop(): void {
    this.endFade();
    this.el.pause();
    this.el.removeAttribute('src');
    this.index = -1;
    this.setStatus('idle');
    this.emitState();
  }

  private setStatus(status: EngineState['status']): void {
    this.status = status;
    if (typeof navigator !== 'undefined' && 'mediaSession' in navigator) {
      navigator.mediaSession.playbackState =
        status === 'playing' ? 'playing' : status === 'paused' ? 'paused' : 'none';
    }
    this.emitState();
  }

  private emitState(): void {
    this.emitter.emit('state', this.getState());
  }

  private wireMediaSession(): void {
    if (typeof navigator === 'undefined' || !('mediaSession' in navigator)) return;
    const ms = navigator.mediaSession;
    ms.setActionHandler('play', () => void this.play());
    ms.setActionHandler('pause', () => void this.pause());
    ms.setActionHandler('previoustrack', () => void this.previous());
    ms.setActionHandler('nexttrack', () => void this.next());
    ms.setActionHandler('seekto', (d) => {
      if (d.seekTime != null) void this.seekTo(d.seekTime);
    });
  }

  private updateMediaMetadata(track: PlayableTrack): void {
    if (typeof window === 'undefined' || !('MediaMetadata' in window)) return;
    if (!('mediaSession' in navigator)) return;
    navigator.mediaSession.metadata = new MediaMetadata({
      title: track.title,
      artist: track.artist ?? '',
      album: track.album ?? '',
      artwork: track.artwork
        ? [{ src: track.artwork, sizes: '512x512', type: 'image/jpeg' }]
        : [],
    });
  }

  private updatePositionState(): void {
    if (typeof navigator === 'undefined' || !('mediaSession' in navigator)) return;
    const setPositionState = navigator.mediaSession.setPositionState?.bind(
      navigator.mediaSession,
    );
    const duration = this.el.duration;
    if (setPositionState && Number.isFinite(duration) && duration > 0) {
      setPositionState({
        duration,
        position: Math.min(this.el.currentTime, duration),
        playbackRate: this.el.playbackRate || 1,
      });
    }
  }
}

export function createEngine(): AudioEngine {
  return new WebAudioEngine();
}
