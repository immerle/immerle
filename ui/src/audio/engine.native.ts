import TrackPlayer, {
  Event,
  PlaybackState,
  PlayerCommand,
  RepeatMode as RNTPRepeatMode,
} from '@rntp/player';
import { EngineEmitter } from './emitter';
import { registerBackgroundHandler, wireRemoteEvents } from './service.native';
import { AudioEngine, EngineState, PlayableTrack, RepeatMode } from './types';

// Register the Android background handler once, at module load, before setup() runs.
registerBackgroundHandler();

const PROGRESS_INTERVAL_MS = 1000;

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

  async setup(): Promise<void> {
    if (this.ready) return;
    TrackPlayer.setupPlayer({ android: { taskRemovedBehavior: 'stop' } });
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
  }

  async setQueue(tracks: PlayableTrack[], startIndex: number): Promise<void> {
    if (!tracks.length) {
      TrackPlayer.clear();
      this.index = -1;
      return;
    }
    const i = Math.max(0, Math.min(startIndex, tracks.length - 1));
    TrackPlayer.setMediaItems(tracks.map(toMediaItem), i);
    this.index = i;
    TrackPlayer.play();
  }

  async add(tracks: PlayableTrack[]): Promise<void> {
    TrackPlayer.addMediaItems(tracks.map(toMediaItem));
  }

  async removeAt(index: number): Promise<void> {
    TrackPlayer.removeMediaItem(index);
  }

  async move(from: number, to: number): Promise<void> {
    TrackPlayer.moveMediaItem(from, to);
  }

  async play(): Promise<void> {
    TrackPlayer.play();
  }

  async pause(): Promise<void> {
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
    TrackPlayer.setRepeatMode(mapRepeat(mode));
  }

  async setVolume(volume: number): Promise<void> {
    TrackPlayer.setVolume(Math.max(0, Math.min(1, volume)));
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
