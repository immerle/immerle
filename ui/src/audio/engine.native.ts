import TrackPlayer, {
  Event,
  PlaybackState,
  PlayerCommand,
  RepeatMode as RNTPRepeatMode,
} from '@rntp/player';
import { Platform } from 'react-native';
import { EngineEmitter } from './emitter';
import { registerBackgroundHandler, wireRemoteEvents } from './service.native';
import { AudioEngine, EngineState, PlayableTrack, RepeatMode } from './types';

// Register the Android background handler once, at module load, before setup()
// runs. It's a documented no-op on iOS (that just logs a dev warning), so skip
// the call there entirely rather than showing a LogBox warning every launch.
if (Platform.OS === 'android') registerBackgroundHandler();

const PROGRESS_INTERVAL_MS = 1000;
// Volume ramp step while fading (finer than the 1s progress poll).
const FADE_TICK_MS = 100;

function mapStatus(state: PlaybackState, playing: boolean): EngineState['status'] {
  switch (state) {
    case PlaybackState.Buffering:
      return 'loading';
    case PlaybackState.Ended:
      return 'ended';
    case PlaybackState.Ready:
      return playing ? 'playing' : 'paused';
    default:
      return 'idle';
  }
}

function mapRepeat(mode: RepeatMode): RNTPRepeatMode {
  switch (mode) {
    case 'track':
      return RNTPRepeatMode.One;
    case 'queue':
      return RNTPRepeatMode.All;
    default:
      return RNTPRepeatMode.Off;
  }
}

function toMediaItem(t: PlayableTrack) {
  return {
    mediaId: t.id,
    url: t.url,
    title: t.title,
    artist: t.artist,
    albumTitle: t.album,
    artworkUrl: t.artwork,
    duration: t.duration,
  };
}

/**
 * Native audio engine backed by @rntp/player: background audio, lockscreen /
 * notification controls, and now-playing metadata come for free from the OS
 * integration. Track-player owns the queue, so this class is mostly a thin,
 * typed adapter that re-broadcasts player events through our emitter.
 *
 * Crossfade: @rntp/player plays one item at a time, so tracks can't overlap.
 * The engine fades the end of a track out and the next one in instead, by
 * ramping the player volume.
 */
class NativeAudioEngine implements AudioEngine {
  private emitter = new EngineEmitter();
  private ready = false;
  private playbackState: PlaybackState = PlaybackState.Idle;
  private playing = false;
  private index = -1;
  private position = 0;
  private duration = 0;
  private progressTimer: ReturnType<typeof setInterval> | null = null;
  private volume = 1;
  private crossfade = 0;
  private repeat: RepeatMode = 'off';
  private fadeTimer: ReturnType<typeof setInterval> | null = null;

  async setup(): Promise<void> {
    if (this.ready) return;
    try {
      TrackPlayer.setupPlayer({ android: { taskRemovedBehavior: 'stop' } });
    } catch (e) {
      // Dev Fast Refresh (or a double-invoked effect) can re-run setup() on a
      // fresh engine instance while the native player singleton from the
      // previous JS load is still set up. @rntp/player throws rather than
      // being idempotent here, so treat that specific case as already-ready.
      if (!(e instanceof Error) || !e.message.includes('already set up')) throw e;
    }
    TrackPlayer.setCommands({
      capabilities: [
        PlayerCommand.PlayPause,
        PlayerCommand.Next,
        PlayerCommand.Previous,
        PlayerCommand.Seek,
        PlayerCommand.Stop,
      ],
    });
    this.wireEvents();
    // Progress cadence isn't native-configurable in v5; poll getProgress()
    // ourselves (same pattern the library's own useProgress hook uses).
    this.progressTimer = setInterval(() => this.pollProgress(), PROGRESS_INTERVAL_MS);
    this.ready = true;
  }

  private wireEvents(): void {
    TrackPlayer.addEventListener(Event.PlaybackStateChanged, ({ state }) => {
      this.playbackState = state;
      this.emitState();
      // End of queue: rewind to the first track and pause (track 1 for a
      // playlist), cursor at the start with the play button ready to replay.
      if (state === PlaybackState.Ended) void this.resetToStart();
    });
    TrackPlayer.addEventListener(Event.IsPlayingChanged, ({ playing }) => {
      this.playing = playing;
      this.emitState();
    });
    TrackPlayer.addEventListener(Event.MediaItemTransition, (e) => {
      // Fade the next track in only after a natural end; a skip starts loud.
      if (this.crossfade && e.reason === 'auto') this.fadeIn();
      else this.stopFade();
      this.index = e.index;
      this.emitter.emit('trackChange', e.index);
      this.emitState();
    });
    wireRemoteEvents();
  }

  private pollProgress(): void {
    const { position, duration } = TrackPlayer.getProgress();
    this.position = position;
    this.duration = duration;
    this.emitter.emit('progress', position, duration);
    if (!this.fadeTimer && this.inFadeOutWindow(position, duration)) {
      this.fadeTimer = setInterval(() => this.fadeOutTick(), FADE_TICK_MS);
    }
  }

  /** Whether playback is within the crossfade window before a natural move to another track. */
  private inFadeOutWindow(position: number, duration: number): boolean {
    if (!this.crossfade || !this.playing || this.repeat === 'track') return false;
    if (duration < this.crossfade * 2 || duration - position > this.crossfade) return false;
    const last = this.index >= TrackPlayer.getQueue().length - 1;
    return !last || this.repeat === 'queue';
  }

  /** Volume follows the time left, so a seek out of the window restores it. */
  private fadeOutTick(): void {
    const { position, duration } = TrackPlayer.getProgress();
    if (!this.inFadeOutWindow(position, duration)) {
      this.stopFade();
      return;
    }
    TrackPlayer.setVolume(this.volume * Math.max(0, (duration - position) / this.crossfade));
  }

  private fadeIn(): void {
    if (this.fadeTimer) clearInterval(this.fadeTimer); // the fade-out hands over at volume ~0
    const start = Date.now();
    const ms = (this.crossfade * 1000) / 2; // a full-length fade-in feels like a quiet intro
    TrackPlayer.setVolume(0);
    this.fadeTimer = setInterval(() => {
      const k = Math.min(1, (Date.now() - start) / ms);
      TrackPlayer.setVolume(this.volume * k);
      if (k >= 1) this.stopFade();
    }, FADE_TICK_MS);
  }

  /** Ends any fade and restores the full volume. */
  private stopFade(): void {
    if (!this.fadeTimer) return;
    clearInterval(this.fadeTimer);
    this.fadeTimer = null;
    TrackPlayer.setVolume(this.volume);
  }

  async setQueue(tracks: PlayableTrack[], startIndex: number): Promise<void> {
    if (!tracks.length) {
      TrackPlayer.clear();
      this.index = -1;
      return;
    }
    const i = Math.max(0, Math.min(startIndex, tracks.length - 1));
    TrackPlayer.setMediaItems(tracks.map(toMediaItem), i);
    // Loads paused, like the web engine (see AudioEngine.setQueue's callers).
    // Must come after: on iOS a non-zero start index goes through the native
    // skipTo, which calls play(), and a reload also keeps the current play
    // state, so audio would otherwise run behind a "paused" UI.
    TrackPlayer.pause();
    this.index = i;
  }

  async add(tracks: PlayableTrack[]): Promise<void> {
    TrackPlayer.addMediaItems(tracks.map(toMediaItem));
  }

  async removeAt(index: number): Promise<void> {
    TrackPlayer.removeMediaItem(index);
  }

  async replaceAt(index: number, track: PlayableTrack): Promise<void> {
    TrackPlayer.replaceMediaItem(index, toMediaItem(track));
  }

  async move(from: number, to: number): Promise<void> {
    TrackPlayer.moveMediaItem(from, to);
  }

  async play(): Promise<void> {
    TrackPlayer.play();
  }

  async pause(): Promise<void> {
    this.stopFade();
    TrackPlayer.pause();
  }

  async skipTo(index: number): Promise<void> {
    TrackPlayer.skipToIndex(index);
    this.index = index;
    TrackPlayer.play();
  }

  async next(): Promise<void> {
    TrackPlayer.skipToNext();
  }

  private async resetToStart(): Promise<void> {
    const queue = TrackPlayer.getQueue();
    if (!queue.length) return;
    TrackPlayer.skipToIndex(0);
    TrackPlayer.seekTo(0);
    TrackPlayer.pause();
    this.index = 0;
  }

  async previous(): Promise<void> {
    // v5 restarts the current item natively when playback is past ~3s, so no
    // manual position check is needed here (unlike the v4/web engines).
    TrackPlayer.skipToPrevious();
  }

  async seekTo(seconds: number): Promise<void> {
    TrackPlayer.seekTo(seconds);
  }

  async setRepeatMode(mode: RepeatMode): Promise<void> {
    this.repeat = mode;
    TrackPlayer.setRepeatMode(mapRepeat(mode));
  }

  async setVolume(volume: number): Promise<void> {
    this.volume = Math.max(0, Math.min(1, volume));
    if (!this.fadeTimer) TrackPlayer.setVolume(this.volume); // a running fade ramps toward it
  }

  async setCrossfade(seconds: number): Promise<void> {
    this.crossfade = Math.max(0, seconds);
    if (!this.crossfade) this.stopFade();
  }

  getState(): EngineState {
    return {
      status: mapStatus(this.playbackState, this.playing),
      index: this.index,
      position: this.position,
      duration: this.duration,
    };
  }

  on: AudioEngine['on'] = (event, handler) => this.emitter.on(event, handler);

  async destroy(): Promise<void> {
    this.stopFade();
    if (this.progressTimer) clearInterval(this.progressTimer);
    this.progressTimer = null;
    TrackPlayer.clear();
    this.ready = false;
    this.playbackState = PlaybackState.Idle;
    this.playing = false;
    this.index = -1;
  }

  private emitState(): void {
    this.emitter.emit('state', this.getState());
  }
}

export function createEngine(): AudioEngine {
  return new NativeAudioEngine();
}
