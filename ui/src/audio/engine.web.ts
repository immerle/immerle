import { EngineEmitter } from './emitter';
import { AudioEngine, EngineState, PlayableTrack, RepeatMode } from './types';

/**
 * Web audio engine: a single `HTMLAudioElement` plus the MediaSession API for
 * OS-level transport controls (play/pause/next/prev and now-playing metadata on
 * the lockscreen / media keys / notification shade).
 *
 * The queue is managed in JS; the element only ever holds the current track.
 * Gapless playback is not attempted here — a single element cannot preload the
 * next source — which matches the "au mieux selon plateforme" requirement.
 */
class WebAudioEngine implements AudioEngine {
  private audio: HTMLAudioElement | null = null;
  private queue: PlayableTrack[] = [];
  private index = -1;
  private repeat: RepeatMode = 'off';
  private emitter = new EngineEmitter();
  private status: EngineState['status'] = 'idle';

  async setup(): Promise<void> {
    if (this.audio) return;
    const audio = new Audio();
    audio.preload = 'auto';
    audio.addEventListener('playing', () => this.setStatus('playing'));
    audio.addEventListener('pause', () => {
      if (this.status !== 'ended') this.setStatus('paused');
    });
    audio.addEventListener('waiting', () => this.setStatus('loading'));
    audio.addEventListener('ended', () => this.handleEnded());
    audio.addEventListener('timeupdate', () => {
      this.emitter.emit('progress', audio.currentTime, audio.duration || 0);
      this.updatePositionState();
    });
    audio.addEventListener('loadedmetadata', () => this.emitState());
    this.audio = audio;
    this.wireMediaSession();
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

  // End of queue: instead of tearing down, rewind to the first track and pause.
  // The cursor sits at the start with the play button ready to replay — for a
  // playlist this means track 1.
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
    this.el.currentTime = seconds;
    this.updatePositionState();
  }

  async setRepeatMode(mode: RepeatMode): Promise<void> {
    this.repeat = mode;
  }

  async setVolume(volume: number): Promise<void> {
    this.el.volume = Math.max(0, Math.min(1, volume));
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

  private handleEnded(): void {
    this.setStatus('ended');
    void this.next();
  }

  private stop(): void {
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
