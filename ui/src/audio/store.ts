import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import { useAuth } from '../auth/store';
import { queryClient } from '../query/queryClient';
import { ImmerleClient } from '../api/immerle/client';
import { toPlayQueueSnapshot } from '../api/immerle/catalog';
import { PlayQueueView } from '../api/immerleApi';
import { PlayQueueSnapshot, Song } from '../api/subsonic/types';
import { offlinePlayableUrl } from '../offline/store';
import { AudioEngine, PlayableTrack, RepeatMode } from './types';
import { createEngine } from './engine';
import { DEFAULT_QUALITY_ID, presetById } from './quality';
import { useToast } from '../stores/toast';
import { t } from '../i18n';

const QUALITY_KEY = 'immerle.quality.v1';
const VOLUME_KEY = 'immerle.volume.v1';

/** Fisher–Yates shuffle (pure). */
function shuffled<T>(arr: T[]): T[] {
  const a = [...arr];
  for (let i = a.length - 1; i > 0; i -= 1) {
    const j = Math.floor(Math.random() * (i + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}

/**
 * Build a player track from a song at the chosen quality. Async because the
 * stream URL is a short-lived signed URL minted from the server (no credential
 * in the URL). The player mints one per track when (re)building the queue.
 *
 * A downloaded track plays from its local file instead — so it works offline and
 * is unaffected by the quality setting (the file is whatever was downloaded;
 * that's also why setQuality's re-mint leaves offline tracks as-is). Artwork
 * still points at the server: covers aren't downloaded, and a broken image is
 * harmless.
 */
async function songToTrack(client: ImmerleClient, song: Song, qualityId: string): Promise<PlayableTrack> {
  // Unresolved (federated-playlist) entries have no playable id yet — resolve
  // them via TrackList's tap flow first. Building the queue must not throw
  // just because one entry, anywhere in the list, isn't resolved: return an
  // empty-url placeholder instead (skipped over on landing — see the
  // `trackChange` handler below).
  if (song.unresolved || !song.id) {
    return { id: '', url: '', title: song.title, artist: song.artist, album: song.album, duration: song.duration };
  }
  const local = await offlinePlayableUrl(song.id);
  let url = local;
  if (!url) {
    const preset = presetById(qualityId);
    url = await client.streamUrl(song.id, {
      maxBitRate: preset.maxBitRate || undefined,
      format: preset.format,
    });
  }
  return {
    id: song.id,
    url,
    title: song.title,
    artist: song.artist,
    album: song.album,
    artwork: client.coverArtUrl(song.coverArt ?? song.id, 512),
    duration: song.duration,
  };
}

/** Per-track scrobble bookkeeping so we submit at most once. */
interface ScrobbleFlags {
  nowPlayingSent: boolean;
  submitted: boolean;
}

interface AudioState {
  engine: AudioEngine | null;
  /** Source songs backing the current queue (for sync & playlist ops). */
  songs: Song[];
  index: number;
  status: 'idle' | 'loading' | 'playing' | 'paused' | 'ended';
  position: number;
  duration: number;
  repeat: RepeatMode;
  shuffle: boolean;
  /** 0..1 playback volume. */
  volume: number;
  qualityId: string;
  /**
   * Server-assigned sole active-playback device id for this account ('' =
   * unrestricted — every device plays independently, today's default). When
   * set to another device, this one should stop driving local audio.
   */
  castTargetId: string;
  /**
   * The device id that last wrote the mirrored state shown here (see
   * applyDisplaySnapshot) — who's actually making the sound right now, even
   * in unrestricted mode where castTargetId is empty. '' when unknown (e.g.
   * this device itself is the source, or nothing's ever been saved).
   */
  playingDeviceId: string;

  init: () => Promise<void>;
  hydrateSettings: () => Promise<void>;

  playSongs: (songs: Song[], startIndex?: number) => Promise<void>;
  /** Play an internet radio station by its raw stream URL (single live track). */
  playRadio: (station: { id: string; name: string; streamUrl: string; hasCover?: boolean }) => Promise<void>;
  playNext: (songs: Song[]) => Promise<void>;
  enqueue: (songs: Song[]) => Promise<void>;
  /** Load a single track by id and seek to a position — used by Jam followers. */
  playTrackById: (id: string, positionSec: number, autoplay: boolean) => Promise<void>;

  toggle: () => Promise<void>;
  next: () => Promise<void>;
  previous: () => Promise<void>;
  seekTo: (seconds: number) => Promise<void>;
  skipTo: (index: number) => Promise<void>;
  removeAt: (index: number) => Promise<void>;
  move: (from: number, to: number) => Promise<void>;
  cycleRepeat: () => Promise<void>;
  toggleShuffle: () => Promise<void>;
  setVolume: (volume: number) => void;
  setQuality: (id: string) => Promise<void>;
  /** Make `deviceId` the sole active player ('' clears it back to independent/"everywhere"). */
  setCastTarget: (deviceId: string) => Promise<void>;

  current: () => Song | undefined;
}

// Module-scoped scrobble state (not reactive UI state).
let scrobble: ScrobbleFlags = { nowPlayingSent: false, submitted: false };
// See scheduleSaveQueue/flushSaveQueue.
let lastQueueSaveAt = 0;
const QUEUE_SAVE_THROTTLE_MS = 5000;
// Original queue order, kept so shuffle can be turned off again.
let orderBackup: Song[] | null = null;
// True while applyRemoteQueue is mid-flight (see there): engine.setQueue
// reloads the audio element, which momentarily reports position 0 before
// the follow-up seekTo corrects it — a stray progress/state event landing
// in that gap must not be treated as this device's real playback position.
let applyingRemoteQueue = false;

function client(): ImmerleClient | null {
  return useAuth.getState().client;
}

// Fallback poll interval for platforms without EventSource (native — see
// connectPlayQueueLive). Web gets real push updates over SSE instead.
const PLAYQUEUE_POLL_MS = 5000;

/** Whether this device is watching another device's session (cast elsewhere). */
function isSpectating(get: () => AudioState): boolean {
  const myId = client()?.getSession()?.deviceId;
  const target = get().castTargetId;
  return !!target && target !== myId;
}

/**
 * If this device is spectating, claim the active-device role before driving
 * the local engine directly — every action that touches `engine` (playSongs,
 * enqueue, ...) must call this first. Without it, while spectating: the
 * local engine is idle/empty (applyDisplaySnapshot never loads anything
 * into it — see its docstring), so any of those actions would either play
 * on top of whatever the actual active device is doing (double audio) or
 * desync from the mirrored queue the store otherwise shows. Matches how
 * Spotify Connect behaves: pressing play on a new thing here takes over
 * from wherever it was playing, rather than just adding a second source.
 * Best-effort/fire-and-forget — the local playback about to start is the
 * real source of truth from this point on regardless of whether the claim
 * itself has landed on the server yet.
 */
function claimActiveDevice(get: () => AudioState, set: (partial: Partial<AudioState>) => void): void {
  if (!isSpectating(get)) return;
  const c = client();
  const myId = c?.getSession()?.deviceId;
  if (!c || !myId) return;
  set({ castTargetId: myId });
  void c.setPlaybackTarget(myId).catch(() => undefined);
}

/**
 * Push a desired current/position/playing state to the server — how a
 * spectator device (see isSpectating) controls the actual active device,
 * which picks the change up on its next poll. Reuses the same write the
 * active device itself uses to sync its own progress (see scheduleSaveQueue);
 * the two are told apart by which device id last wrote it.
 */
function sendRemoteCommand(get: () => AudioState, current: string, positionMs: number, playing: boolean): void {
  const c = client();
  const { songs } = get();
  if (!c || !songs.length) return;
  void c
    .savePlayQueue(
      songs.map((s) => s.id),
      current,
      positionMs,
      playing,
    )
    .catch(() => undefined);
}

/**
 * Load a saved server-side queue into local state and the engine, at its
 * saved position — shared by the launch restore and by "this device is (or
 * just became) the active player". Always lands paused unless `autoplay`,
 * matching playTrackById's pattern for a single-track load.
 */
async function applyRemoteQueue(
  get: () => AudioState,
  set: (partial: Partial<AudioState>) => void,
  remote: PlayQueueSnapshot,
  autoplay: boolean,
): Promise<void> {
  const c = client();
  const engine = get().engine;
  if (!c || !engine || !remote.songs.length) return;
  const idx = Math.max(0, remote.songs.findIndex((s) => s.id === remote.currentId));
  orderBackup = null;
  scrobble = { nowPlayingSent: false, submitted: false };
  set({ songs: remote.songs, index: idx, position: remote.positionMs / 1000, playingDeviceId: client()?.getSession()?.deviceId ?? '' });
  applyingRemoteQueue = true;
  try {
    const tracks = await Promise.all(remote.songs.map((s) => songToTrack(c, s, get().qualityId)));
    await engine.setQueue(tracks, idx);
    await engine.seekTo(remote.positionMs / 1000);
    if (!autoplay) await engine.pause();
  } finally {
    // Keep suppressing engine-driven writes for a short grace period past
    // the awaited calls above settling: a browser audio element's own
    // pause/playing/waiting events (from the reload inside setQueue) don't
    // necessarily fire in lockstep with those promises resolving — a
    // trailing one landing right as the guard lifts would otherwise
    // override the reassertion below with a stale mid-transition value
    // (this is what made a spectator's seek/remote command flip the play
    // button back to "paused" even though playback carried on correctly).
    setTimeout(() => {
      applyingRemoteQueue = false;
    }, 500);
  }
  // Re-assert: the engine's own state/progress events were dropped, not
  // corrected, during the guarded window above (status would otherwise stay
  // whatever it was before this call — engine.setQueue/pause's own events
  // never landed).
  set({ position: remote.positionMs / 1000, status: autoplay ? 'playing' : 'paused' });
  sendNowPlaying(get);
  // Immediately, not throttled: this is the moment this device reclaims the
  // shared queue (changedBy becomes this device's id), so a stale write from
  // whoever it took over from doesn't keep getting re-applied on every poll.
  flushSaveQueue(get);
}

/**
 * Mirror a saved queue into local state for display only — used whenever
 * another device is (or might be) the one actually producing sound, so this
 * device never starts audio just because it noticed a change. songs/index
 * still get set (not just cosmetic text) so a spectator's transport taps
 * (see isSpectating) have something to compute the next command against.
 */
function applyDisplaySnapshot(set: (partial: Partial<AudioState>) => void, remote: PlayQueueSnapshot): void {
  if (!remote.songs.length) return; // nothing saved yet — leave whatever's already shown alone
  const idx = Math.max(0, remote.songs.findIndex((s) => s.id === remote.currentId));
  set({
    songs: remote.songs,
    index: idx,
    position: remote.positionMs / 1000,
    duration: remote.songs[idx]?.duration ?? 0,
    status: remote.playing ? 'playing' : 'paused',
    // Prefer the explicit cast target (who's supposed to be playing) over
    // changedBy (who last wrote) — the target stays authoritative even for
    // one poll/event cycle where a fresh write hasn't landed from them yet.
    playingDeviceId: remote.targetDeviceId || remote.changedBy || '',
  });
}

/**
 * Restore the last-known cross-device state at launch. Loads it into the
 * engine (paused, ready to hit play) unless another device is explicitly the
 * active target, in which case it's display-only — this device shouldn't
 * start buffering audio it was never asked to play.
 */
async function restoreQueue(get: () => AudioState, set: (partial: Partial<AudioState>) => void): Promise<void> {
  const c = client();
  if (!c || !get().engine) return;
  const remote = await c.getPlayQueue().catch(() => null);
  if (!remote) return;
  set({ castTargetId: remote.targetDeviceId });
  const myId = c.getSession()?.deviceId;
  if (remote.targetDeviceId && remote.targetDeviceId !== myId) {
    applyDisplaySnapshot(set, remote);
  } else {
    await applyRemoteQueue(get, set, remote, false);
  }
}

/**
 * Reconciles this device against a freshly-received queue snapshot (from the
 * SSE stream, or a poll on platforms without it). The explicit active device
 * (target === my id) applies real playback changes — that's how a
 * spectator's remote command (see sendRemoteCommand) actually reaches it.
 * Every other device (spectating someone else, or in the default
 * unrestricted mode) only ever mirrors state for display; it never starts
 * local audio on its own say-so.
 */
async function reconcilePlayQueue(
  get: () => AudioState,
  set: (partial: Partial<AudioState>) => void,
  remote: PlayQueueSnapshot,
): Promise<void> {
  const engine = get().engine;
  const myId = client()?.getSession()?.deviceId;
  const target = remote.targetDeviceId;
  // eslint-disable-next-line no-console
  console.log('[playqueue] reconcile', {
    myId,
    target,
    changedBy: remote.changedBy,
    current: remote.currentId,
    playing: remote.playing,
    hasEngine: !!engine,
  });
  if (!engine) return;
  set({ castTargetId: target });

  if (target && target === myId) {
    // I'm explicitly the active device. Only react to a write that isn't my
    // own — re-applying my own (possibly lagging) save would revert a more
    // recent local change made since that save went out.
    if (remote.changedBy && remote.changedBy !== myId) {
      // eslint-disable-next-line no-console
      console.log('[playqueue] taking over (remote command applied)');
      await applyRemoteQueue(get, set, remote, remote.playing);
    }
    return;
  }

  if (target && get().status === 'playing') await engine.pause(); // handed off elsewhere — avoid double audio
  applyDisplaySnapshot(set, remote);
}

/**
 * Live-updates this device on every play-queue change: Server-Sent Events on
 * platforms that have them (web), a short poll everywhere else (native —
 * React Native has no EventSource, and adding an SSE polyfill for one
 * feature isn't worth the dependency). Same reconciliation either way — see
 * reconcilePlayQueue. EventSource reconnects on its own, so no retry logic
 * is needed for the SSE path.
 */
function connectPlayQueueLive(get: () => AudioState, set: (partial: Partial<AudioState>) => void): void {
  const c = client();
  if (!c) return;
  const ES = (globalThis as { EventSource?: new (url: string) => EventSourceLike }).EventSource;
  if (ES) {
    const url = c.playQueueEventsUrl();
    // eslint-disable-next-line no-console
    console.log('[playqueue] connecting SSE', url.replace(/apiKey=[^&]+/, 'apiKey=***'));
    const es = new ES(url);
    es.addEventListener('open', () => {
      // eslint-disable-next-line no-console
      console.log('[playqueue] SSE open');
    });
    es.addEventListener('error', (e) => {
      // eslint-disable-next-line no-console
      console.warn('[playqueue] SSE error (browser will auto-reconnect)', e);
    });
    es.addEventListener('state', (e: { data?: string }) => {
      // eslint-disable-next-line no-console
      console.log('[playqueue] SSE message', e.data);
      if (!e.data) return;
      try {
        const view = JSON.parse(e.data) as PlayQueueView;
        void reconcilePlayQueue(get, set, toPlayQueueSnapshot(view));
      } catch (err) {
        // eslint-disable-next-line no-console
        console.warn('[playqueue] failed to parse SSE event', err);
      }
    });
    return;
  }
  const poll = () =>
    client()
      ?.getPlayQueue()
      .then((remote) => reconcilePlayQueue(get, set, remote))
      .catch(() => undefined);
  setInterval(() => void poll(), PLAYQUEUE_POLL_MS);
}

/**
 * While spectating and the mirrored session is playing, position only moves
 * in jumps — once per update (SSE push, or the native poll interval), which
 * can be several seconds apart, reading as the progress bar visibly
 * skipping. Tick it forward locally once a second in between so it moves
 * smoothly instead; every real update (applyDisplaySnapshot) still
 * overwrites it with the authoritative value regardless, so this can never
 * drift for more than a second or two.
 */
function startFakeProgressTicker(get: () => AudioState, set: (partial: Partial<AudioState>) => void): void {
  setInterval(() => {
    if (!isSpectating(get) || get().status !== 'playing') return;
    const { position, duration } = get();
    set({ position: duration > 0 ? Math.min(position + 1, duration) : position + 1 });
  }, 1000);
}

interface EventSourceLike {
  addEventListener: (type: string, listener: (e: { data?: string }) => void) => void;
}

/**
 * If a remote track's background download has finished, swap it in place for
 * the resolved local song so it becomes seekable. Returns whether the swap
 * happened. Re-checks `index`/`song` after the network round trip in case the
 * queue moved on while the check was in flight.
 */
async function upgradeIfDownloaded(
  get: () => AudioState,
  set: (partial: Partial<AudioState>) => void,
  index: number,
  song: Song,
): Promise<boolean> {
  const c = client();
  const engine = get().engine;
  if (!c || !engine) return false;
  const status = await c.getSongLocalStatus(song.id).catch(() => null);
  if (!status?.local || !status.song) return false;
  if (get().index !== index || get().songs[index]?.id !== song.id) return false;
  const songs = [...get().songs];
  songs[index] = status.song;
  set({ songs });
  await engine.replaceAt(index, await songToTrack(c, status.song, get().qualityId));
  return true;
}

export const usePlayer = create<AudioState>((set, get) => ({
  engine: null,
  songs: [],
  index: -1,
  status: 'idle',
  position: 0,
  duration: 0,
  repeat: 'off',
  shuffle: false,
  volume: 1,
  qualityId: DEFAULT_QUALITY_ID,
  castTargetId: '',
  playingDeviceId: '',

  hydrateSettings: async () => {
    try {
      const q = await AsyncStorage.getItem(QUALITY_KEY);
      if (q) set({ qualityId: q });
      const v = await AsyncStorage.getItem(VOLUME_KEY);
      if (v !== null) set({ volume: Math.max(0, Math.min(1, Number(v))) });
    } catch {
      /* keep defaults */
    }
  },

  init: async () => {
    // eslint-disable-next-line no-console
    console.log('[playqueue] init() called', { alreadyHasEngine: !!get().engine });
    if (get().engine) return;
    const engine = createEngine();
    await engine.setup();

    // While spectating (isSpectating), the local engine sits idle — it's
    // never given anything to play (applyDisplaySnapshot deliberately never
    // touches it, see its docstring). But it can still fire a stray
    // state/progress event on the way to idle (e.g. right after the poll's
    // own engine.pause() call above resolves), and without this guard that
    // event would win the race and stomp the mirrored remote status/position
    // right back to this device's own (idle) values — the player bar and
    // progress bar would then show the wrong thing until the next update.
    engine.on('state', (s) => {
      if (isSpectating(get) || applyingRemoteQueue) return;
      const wasPlaying = get().status === 'playing';
      set({ status: s.status, index: s.index, duration: s.duration || get().duration });
      // Capture a pause immediately (not throttled) — it stops the progress
      // ticks that would otherwise eventually trigger a save, so without
      // this the paused position/state could sit unsaved indefinitely.
      if (wasPlaying && s.status !== 'playing') flushSaveQueue(get);
    });
    engine.on('progress', (position, duration) => {
      if (isSpectating(get) || applyingRemoteQueue) return;
      set({ position, duration: duration || get().duration });
      maybeScrobble(get, position, duration);
      scheduleSaveQueue(get);
    });
    engine.on('trackChange', (index) => {
      if (isSpectating(get) || applyingRemoteQueue) return;
      scrobble = { nowPlayingSent: false, submitted: false };
      set({ index, position: 0 });
      sendNowPlaying(get);
      flushSaveQueue(get);
      // Landed on a federated-playlist track that hasn't been resolved yet
      // (empty-url placeholder — see songToTrack): warn instead of silently
      // sitting on dead air, and move on. ponytail: if the whole queue is
      // unresolved this cascades toast-after-toast to the end (or forever,
      // with repeat on) — fine for the size of federated playlists today.
      const song = get().songs[index];
      if (song?.unresolved) {
        useToast.getState().warning(t('media.player.unresolvedSkipped', { title: song.title }));
        void get().engine?.next();
      }
    });

    await engine.setVolume(get().volume);
    set({ engine });
    startFakeProgressTicker(get, set);

    // Cross-device state: show what's already playing (paused) on launch,
    // then stay live-updated on every change from any device (see
    // connectPlayQueueLive) — someone else starting/pausing/skipping, or
    // handing playback to/from this one. init() runs in the same effect as
    // useAuth's restore(), racing it — client() is still null the vast
    // majority of the time at this exact point, since restoring a session
    // is async (secure storage read, capability probe, account fetch).
    // Both restoreQueue and connectPlayQueueLive silently no-op without a
    // client and, unlike a user-triggered action, nothing would otherwise
    // call them again — so wait for the client to actually exist first.
    const startLiveSync = () => {
      // eslint-disable-next-line no-console
      console.log('[playqueue] starting live sync', { deviceId: client()?.getSession()?.deviceId });
      void restoreQueue(get, set);
      connectPlayQueueLive(get, set);
    };
    if (client()) {
      // eslint-disable-next-line no-console
      console.log('[playqueue] client already ready at init()');
      startLiveSync();
    } else {
      // eslint-disable-next-line no-console
      console.log('[playqueue] client not ready yet, waiting for auth');
      const unsub = useAuth.subscribe((s) => {
        if (s.client) {
          unsub();
          startLiveSync();
        }
      });
    }

    // Best-effort final save when the tab is backgrounded/closed (web only —
    // document is undefined on native, where the app-background lifecycle
    // doesn't give the same "about to lose this" moment). Catches state that
    // hasn't hit the 5s throttle yet.
    if (typeof document !== 'undefined' && typeof document.addEventListener === 'function') {
      document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') flushSaveQueue(get);
      });
    }
  },

  playSongs: async (songs, startIndex = 0) => {
    const c = client();
    const engine = get().engine;
    if (!c || !engine || songs.length === 0) return;
    claimActiveDevice(get, set);
    const tracks = await Promise.all(songs.map((s) => songToTrack(c, s, get().qualityId)));
    orderBackup = null; // new playback context invalidates any shuffle backup
    set({ songs, index: startIndex, position: 0 });
    scrobble = { nowPlayingSent: false, submitted: false };
    await engine.setQueue(tracks, startIndex);
    sendNowPlaying(get);
    flushSaveQueue(get);
  },

  playRadio: async (station) => {
    const engine = get().engine;
    if (!engine || !station.streamUrl) return;
    claimActiveDevice(get, set);
    // Live streams aren't scrobbled and have no real duration. The raw URL is
    // played directly (not routed through the Subsonic stream endpoint).
    const track: PlayableTrack = { id: station.id, url: station.streamUrl, title: station.name, artist: '', duration: 0 };
    const c = client();
    const coverUrl = station.hasCover && c ? c.radioCoverUrl(station.id) : undefined;
    const song = { id: station.id, title: station.name, artist: '', coverUrl } as Song;
    orderBackup = null;
    scrobble = { nowPlayingSent: true, submitted: true };
    set({ songs: [song], index: 0, position: 0 });
    await engine.setQueue([track], 0);
  },

  playTrackById: async (id, positionSec, autoplay) => {
    const c = client();
    const engine = get().engine;
    if (!c || !engine) return;
    claimActiveDevice(get, set);
    const song = await c.getSong(id).catch(() => ({ id, title: 'Piste' }) as Song);
    orderBackup = null;
    scrobble = { nowPlayingSent: false, submitted: false };
    set({ songs: [song], index: 0, position: positionSec });
    await engine.setQueue([await songToTrack(c, song, get().qualityId)], 0);
    await engine.seekTo(positionSec);
    if (!autoplay) await engine.pause();
    sendNowPlaying(get);
  },

  playNext: async (songs) => {
    const c = client();
    const engine = get().engine;
    if (!c || !engine) return get().playSongs(songs);
    if (get().songs.length === 0) return get().playSongs(songs);
    // ponytail: while spectating this only claims the device — it doesn't
    // reload the engine with the mirrored queue first, so the insert below
    // still targets whatever the (idle, out of sync) local engine already
    // has. Fine for the common case (claim, then use the transport
    // normally); a genuine "insert next" mid-spectate is a rarer path to
    // get fully right and isn't what was reported.
    claimActiveDevice(get, set);
    // Insert right after the current track in our source mirror + engine queue.
    const at = get().index + 1;
    const next = [...get().songs];
    next.splice(at, 0, ...songs);
    set({ songs: next });
    // Engine has no insert-at; append then move into place.
    await engine.add(await Promise.all(songs.map((s) => songToTrack(c, s, get().qualityId))));
    for (let i = 0; i < songs.length; i += 1) {
      await engine.move(get().songs.length - 1, at + i);
    }
  },

  enqueue: async (songs) => {
    const c = client();
    const engine = get().engine;
    if (!c || !engine) return;
    if (get().songs.length === 0) return get().playSongs(songs);
    claimActiveDevice(get, set);
    set({ songs: [...get().songs, ...songs] });
    await engine.add(await Promise.all(songs.map((s) => songToTrack(c, s, get().qualityId))));
  },

  toggle: async () => {
    if (isSpectating(get)) {
      const { songs, index, position, status } = get();
      const song = songs[index];
      if (!song) return;
      const playing = status !== 'playing';
      set({ status: playing ? 'playing' : 'paused' }); // optimistic; the active device converges on its next poll
      sendRemoteCommand(get, song.id, Math.floor(position * 1000), playing);
      return;
    }
    const engine = get().engine;
    if (!engine) return;
    if (get().status === 'playing') await engine.pause();
    else await engine.play();
  },

  next: async () => {
    if (isSpectating(get)) {
      const { songs, index } = get();
      const song = songs[index + 1];
      if (!song) return;
      set({ index: index + 1, position: 0, status: 'playing' });
      sendRemoteCommand(get, song.id, 0, true);
      return;
    }
    await get().engine?.next();
  },

  previous: async () => {
    if (isSpectating(get)) {
      const { songs, index } = get();
      const song = songs[index - 1];
      if (!song) return;
      set({ index: index - 1, position: 0, status: 'playing' });
      sendRemoteCommand(get, song.id, 0, true);
      return;
    }
    await get().engine?.previous();
  },

  seekTo: async (seconds) => {
    if (isSpectating(get)) {
      const { songs, index, status } = get();
      const song = songs[index];
      if (!song) return;
      set({ position: seconds }); // optimistic
      sendRemoteCommand(get, song.id, Math.floor(seconds * 1000), status === 'playing');
      return;
    }
    const engine = get().engine;
    if (!engine) return;
    // A not-yet-downloaded track streams progressively (see songToTrack): the
    // server can't serve byte ranges for it yet, so a seek would silently
    // restart playback from 0. If the background download has since
    // finished, swap in the now-local (seekable) track first — otherwise
    // bail out with a toast, same as before this check existed. Guarded here
    // (not just in the UI) so an OS media-session seek control (lock screen /
    // headset) can't trigger a raw seek either.
    const index = get().index;
    const song = get().songs[index];
    if (song?.remote && !(await upgradeIfDownloaded(get, set, index, song))) {
      useToast.getState().warning(t('media.player.seekUnavailableRemote'));
      return;
    }
    await engine.seekTo(seconds);
    set({ position: seconds });
  },

  skipTo: async (index) => {
    if (isSpectating(get)) {
      const song = get().songs[index];
      if (!song) return;
      set({ index, position: 0, status: 'playing' });
      sendRemoteCommand(get, song.id, 0, true);
      return;
    }
    await get().engine?.skipTo(index);
  },

  removeAt: async (index) => {
    const engine = get().engine;
    if (!engine) return;
    const songs = [...get().songs];
    songs.splice(index, 1);
    set({ songs });
    await engine.removeAt(index);
  },

  move: async (from, to) => {
    const engine = get().engine;
    if (!engine) return;
    const songs = [...get().songs];
    const [item] = songs.splice(from, 1);
    if (item) songs.splice(to, 0, item);
    set({ songs });
    await engine.move(from, to);
  },

  cycleRepeat: async () => {
    const order: RepeatMode[] = ['off', 'queue', 'track'];
    const next = order[(order.indexOf(get().repeat) + 1) % order.length];
    set({ repeat: next });
    await get().engine?.setRepeatMode(next);
  },

  toggleShuffle: async () => {
    const c = client();
    const engine = get().engine;
    if (!c || !engine) {
      set({ shuffle: !get().shuffle });
      return;
    }
    const { songs, index, position, shuffle } = get();
    const current = songs[index];

    let nextSongs: Song[];
    let nextIndex: number;
    if (!shuffle) {
      // Enable: keep the current track, shuffle everything else as upcoming.
      orderBackup = [...songs];
      const rest = songs.filter((_, i) => i !== index);
      nextSongs = current ? [current, ...shuffled(rest)] : shuffled(rest);
      nextIndex = current ? 0 : Math.max(0, index);
    } else {
      // Disable: restore the original order, landing on the current track.
      const base = orderBackup ?? songs;
      orderBackup = null;
      const found = current ? base.findIndex((s) => s.id === current.id) : -1;
      nextSongs = base;
      nextIndex = found >= 0 ? found : Math.max(0, index);
    }

    set({ shuffle: !shuffle, songs: nextSongs, index: nextIndex });
    if (nextSongs.length === 0) return;
    // Rebuild the engine queue around the (unchanged) current track, then
    // restore the playback position so the music doesn't visibly restart.
    const tracks = await Promise.all(nextSongs.map((s) => songToTrack(c, s, get().qualityId)));
    await engine.setQueue(tracks, nextIndex);
    await engine.seekTo(position);
  },

  setVolume: (volume) => {
    const v = Math.max(0, Math.min(1, volume));
    set({ volume: v });
    void get().engine?.setVolume(v);
    void AsyncStorage.setItem(VOLUME_KEY, String(v));
  },

  setQuality: async (id) => {
    set({ qualityId: id });
    void AsyncStorage.setItem(QUALITY_KEY, id);
    // Re-derive URLs for the remaining queue at the new quality without losing
    // the current position.
    const c = client();
    const engine = get().engine;
    if (!c || !engine || get().songs.length === 0) return;
    const pos = get().position;
    const idx = get().index;
    const tracks = await Promise.all(get().songs.map((s) => songToTrack(c, s, id)));
    await engine.setQueue(tracks, idx);
    await engine.seekTo(pos);
  },

  setCastTarget: async (deviceId) => {
    const c = client();
    if (!c) return;
    try {
      await c.setPlaybackTarget(deviceId);
    } catch {
      return; // best-effort; UI keeps its previous state
    }
    set({ castTargetId: deviceId });
    if (!deviceId) return; // cleared — independent mode, no forced action
    const myId = c.getSession()?.deviceId;
    if (deviceId === myId) {
      const remote = await c.getPlayQueue().catch(() => null);
      if (remote) await applyRemoteQueue(get, set, remote, true);
      return;
    }
    if (get().status === 'playing') await get().engine?.pause();
    const remote = await c.getPlayQueue().catch(() => null);
    if (remote) applyDisplaySnapshot(set, remote); // show the new target's state right away, don't wait for the next poll
  },

  current: () => {
    const { songs, index } = get();
    return index >= 0 ? songs[index] : undefined;
  },
}));

// --- Scrobbling & queue sync (side effects) --------------------------------

function sendNowPlaying(get: () => AudioState): void {
  const c = client();
  const song = get().current();
  if (!c || !song || scrobble.nowPlayingSent) return;
  scrobble.nowPlayingSent = true;
  void c.scrobble(song.id, false).catch(() => undefined);
}

/** Submit a real scrobble once playback passes the half / 4-minute mark. */
function maybeScrobble(get: () => AudioState, position: number, duration: number): void {
  if (scrobble.submitted) return;
  const threshold = duration > 0 ? Math.min(duration / 2, 240) : 240;
  if (position < threshold) return;
  const c = client();
  const song = get().current();
  if (!c || !song) return;
  scrobble.submitted = true;
  void c
    .scrobble(song.id, true)
    .then(() => {
      // The play now counts: refresh the "recently played" / "most played"
      // home rows so the album surfaces without a manual reload.
      queryClient.invalidateQueries({ queryKey: ['albumList', 'recent'] });
      queryClient.invalidateQueries({ queryKey: ['albumList', 'frequent'] });
    })
    .catch(() => undefined);
}

/**
 * Immediately persists the current queue/track/position/playing state. Never
 * writes while spectating (mirroring another device — see isSpectating):
 * its local songs/index/status are a copy of what that device already told
 * the server, so writing them back here would just churn `changedBy` to this
 * device's id for no real change, tricking the active device's poll into
 * thinking a genuine command arrived and needlessly re-applying (restarting)
 * its own playback. Deliberate remote commands (sendRemoteCommand) bypass
 * this and always write — they're the one case a spectator legitimately
 * changes the shared session.
 */
function flushSaveQueue(get: () => AudioState): void {
  if (isSpectating(get)) return;
  lastQueueSaveAt = Date.now();
  const c = client();
  const { songs, index, position, status } = get();
  if (!c || songs.length === 0 || index < 0) return;
  void c
    .savePlayQueue(
      songs.map((s) => s.id),
      songs[index]?.id,
      Math.floor(position * 1000),
      status === 'playing',
    )
    .catch(() => undefined);
}

/**
 * Throttled savePlayQueue for the high-frequency progress tick (fires every
 * ~250ms-1s during playback). A plain debounce here — reset on every call —
 * would never actually land during continuous playback, since each tick
 * cancels the pending one before it fires: the server-side queue would stay
 * stale until playback stops, losing state entirely if the tab/app closes
 * mid-song. Save at most once every 5s instead; discrete transitions (pause,
 * track change, taking over a cast) call flushSaveQueue directly so those
 * moments are captured immediately rather than waiting out the throttle.
 */
function scheduleSaveQueue(get: () => AudioState): void {
  if (Date.now() - lastQueueSaveAt < QUEUE_SAVE_THROTTLE_MS) return;
  flushSaveQueue(get);
}
