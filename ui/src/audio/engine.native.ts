import TrackPlayer, {
  AppKilledPlaybackBehavior,
  Capability,
  Event,
  RepeatMode as RNTPRepeatMode,
  State,
} from 'react-native-track-player';
import { EngineEmitter } from './emitter';
import { PlaybackService } from './service.native';
import { AudioEngine, EngineState, PlayableTrack, RepeatMode } from './types';

// Register the playback service once, at module load, before setup() runs.
TrackPlayer.registerPlaybackService(() => PlaybackService);

function mapState(state: State | undefined): EngineState['status'] {
  switch (state) {
    case State.Playing:
      return 'playing';
    case State.Paused:
    case State.Ready:
      return 'paused';
    case State.Buffering:
    case State.Loading:
      return 'loading';
    case State.Ended:
      return 'ended';
    default:
      return 'idle';
  }
}

function mapRepeat(mode: RepeatMode): RNTPRepeatMode {
  switch (mode) {
    case 'track':
      return RNTPRepeatMode.Track;
    case 'queue':
      return RNTPRepeatMode.Queue;
    default:
      return RNTPRepeatMode.Off;
  }
}

function toRNTPTrack(t: PlayableTrack) {
  return {
    id: t.id,
    url: t.url,
    title: t.title,
    artist: t.artist,
    album: t.album,
    artwork: t.artwork,
    duration: t.duration,
  };
}

/**
 * Native audio engine backed by react-native-track-player: background audio,
 * lockscreen / notification controls, and now-playing metadata come for free
 * from the OS integration. Track-player owns the queue, so this class is mostly
 * a thin, typed adapter that re-broadcasts player events through our emitter.
 */
class NativeAudioEngine implements AudioEngine {
  private emitter = new EngineEmitter();
  private ready = false;
  private status: EngineState['status'] = 'idle';
  private index = -1;
  private position = 0;
  private duration = 0;

  async setup(): Promise<void> {
    if (this.ready) return;
    await TrackPlayer.setupPlayer({ autoHandleInterruptions: true });
    await TrackPlayer.updateOptions({
      android: {
        appKilledPlaybackBehavior: AppKilledPlaybackBehavior.StopPlaybackAndRemoveNotification,
      },
      capabilities: [
        Capability.Play,
        Capability.Pause,
        Capability.SkipToNext,
        Capability.SkipToPrevious,
        Capability.SeekTo,
        Capability.Stop,
      ],
      compactCapabilities: [Capability.Play, Capability.Pause, Capability.SkipToNext],
      progressUpdateEventInterval: 1,
    });
    this.wireEvents();
    this.ready = true;
  }

  private wireEvents(): void {
    TrackPlayer.addEventListener(Event.PlaybackState, ({ state }) => {
      this.status = mapState(state);
      this.emitState();
      // End of queue: rewind to the first track and pause (track 1 for a
      // playlist), cursor at the start with the play button ready to replay.
      if (state === State.Ended) void this.resetToStart();
    });
    TrackPlayer.addEventListener(Event.PlaybackActiveTrackChanged, (e) => {
      if (typeof e.index === 'number') {
        this.index = e.index;
        this.emitter.emit('trackChange', e.index);
        this.emitState();
      }
    });
    TrackPlayer.addEventListener(Event.PlaybackProgressUpdated, (e) => {
      this.position = e.position;
      this.duration = e.duration;
      this.emitter.emit('progress', e.position, e.duration);
    });
  }

  async setQueue(tracks: PlayableTrack[], startIndex: number): Promise<void> {
    await TrackPlayer.reset();
    await TrackPlayer.add(tracks.map(toRNTPTrack));
    if (tracks.length) {
      const i = Math.max(0, Math.min(startIndex, tracks.length - 1));
      await TrackPlayer.skip(i);
      this.index = i;
      await TrackPlayer.play();
    }
  }

  async add(tracks: PlayableTrack[]): Promise<void> {
    await TrackPlayer.add(tracks.map(toRNTPTrack));
  }

  async removeAt(index: number): Promise<void> {
    await TrackPlayer.remove([index]);
  }

  async move(from: number, to: number): Promise<void> {
    await TrackPlayer.move(from, to);
  }

  async play(): Promise<void> {
    await TrackPlayer.play();
  }

  async pause(): Promise<void> {
    await TrackPlayer.pause();
  }

  async skipTo(index: number): Promise<void> {
    await TrackPlayer.skip(index);
    this.index = index;
    await TrackPlayer.play();
  }

  async next(): Promise<void> {
    await TrackPlayer.skipToNext().catch(() => undefined);
  }

  private async resetToStart(): Promise<void> {
    const queue = await TrackPlayer.getQueue();
    if (!queue.length) return;
    await TrackPlayer.skip(0);
    await TrackPlayer.seekTo(0);
    await TrackPlayer.pause();
    this.index = 0;
  }

  async previous(): Promise<void> {
    const { position } = await TrackPlayer.getProgress();
    if (position > 3) {
      await TrackPlayer.seekTo(0);
      return;
    }
    await TrackPlayer.skipToPrevious().catch(() => undefined);
  }

  async seekTo(seconds: number): Promise<void> {
    await TrackPlayer.seekTo(seconds);
  }

  async setRepeatMode(mode: RepeatMode): Promise<void> {
    await TrackPlayer.setRepeatMode(mapRepeat(mode));
  }

  async setVolume(volume: number): Promise<void> {
    await TrackPlayer.setVolume(Math.max(0, Math.min(1, volume)));
  }

  getState(): EngineState {
    return {
      status: this.status,
      index: this.index,
      position: this.position,
      duration: this.duration,
    };
  }

  on: AudioEngine['on'] = (event, handler) => this.emitter.on(event, handler);

  async destroy(): Promise<void> {
    await TrackPlayer.reset();
    this.ready = false;
    this.status = 'idle';
    this.index = -1;
  }

  private emitState(): void {
    this.emitter.emit('state', this.getState());
  }
}

export function createEngine(): AudioEngine {
  return new NativeAudioEngine();
}
