/**
 * Platform-agnostic audio contract.
 *
 * Two engines implement {@link AudioEngine}: `engine.native.ts`
 * (react-native-track-player — background playback, lockscreen/notification
 * controls, now-playing) and `engine.web.ts` (HTMLAudioElement + MediaSession).
 * The store in `store.ts` talks only to this interface, never to a concrete
 * engine, so the rest of the app is platform-blind.
 */

export interface PlayableTrack {
  /** Subsonic song id. */
  id: string;
  /** Fully-authenticated stream URL (already transcoded per quality). */
  url: string;
  title: string;
  artist?: string;
  album?: string;
  /** Cover-art URL for lockscreen / MediaSession artwork. */
  artwork?: string;
  /** Duration in seconds, if known. */
  duration?: number;
}

export type PlaybackStatus = 'idle' | 'loading' | 'playing' | 'paused' | 'ended';
export type RepeatMode = 'off' | 'track' | 'queue';

/** Snapshot pushed from the engine to the store on every meaningful change. */
export interface EngineState {
  status: PlaybackStatus;
  /** Index into the engine's queue, or -1 when idle. */
  index: number;
  /** Current position in seconds. */
  position: number;
  /** Active track duration in seconds. */
  duration: number;
}

export interface EngineEvents {
  /** Fired on play/pause/load/end and on track transitions. */
  state: (state: EngineState) => void;
  /** High-frequency position updates (≈ once per second). */
  progress: (position: number, duration: number) => void;
  /** A track finished and the engine advanced; carries the new index. */
  trackChange: (index: number) => void;
}

export interface AudioEngine {
  /** One-time setup (registers the playback service / media session). */
  setup(): Promise<void>;

  /**
   * Replace the queue, loading `startIndex` — paused. Does not itself start
   * playback: callers that want it playing call play() explicitly afterward,
   * once any seekTo() they need has also been applied. That ordering matters:
   * calling play() before a caller-requested seek raced the seek against
   * playback already starting from 0, which on the web engine could leave
   * the seek silently dropped (see engine.web.ts).
   */
  setQueue(tracks: PlayableTrack[], startIndex: number): Promise<void>;

  /** Append tracks to the end of the queue. */
  add(tracks: PlayableTrack[]): Promise<void>;

  /** Remove the queue entry at `index`. */
  removeAt(index: number): Promise<void>;

  /**
   * Swap the queue entry at `index` for a freshly-resolved track (e.g. a
   * remote track that has finished downloading in the background), without
   * disturbing the rest of the queue. Best-effort resumes playback if that
   * index was playing.
   */
  replaceAt(index: number, track: PlayableTrack): Promise<void>;

  /** Move a queue entry (for drag-to-reorder). */
  move(from: number, to: number): Promise<void>;

  play(): Promise<void>;
  pause(): Promise<void>;
  /** Jump to a queue index and play. */
  skipTo(index: number): Promise<void>;
  next(): Promise<void>;
  previous(): Promise<void>;
  /** Seek within the current track, in seconds. */
  seekTo(seconds: number): Promise<void>;

  setRepeatMode(mode: RepeatMode): Promise<void>;
  setVolume(volume: number): Promise<void>;

  getState(): EngineState;

  /** Subscribe to an engine event. Returns an unsubscribe function. */
  on<E extends keyof EngineEvents>(event: E, handler: EngineEvents[E]): () => void;

  /** Tear down (web: release the audio element; native: reset track-player). */
  destroy(): Promise<void>;
}
