import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import { useAuth } from '../auth/store';
import { queryClient } from '../query/queryClient';
import { ImmerleClient, toJamInvite } from '../api/immerle/client';
import { toPlayQueueSnapshot } from '../api/immerle/catalog';
import { JamInviteDTO, PlayQueueView } from '../api/immerleApi';
import { PlayQueueCommand, PlayQueueSnapshot, Song } from '../api/subsonic/types';
import { offlinePlayableUrl } from '../offline/store';
import { AudioEngine, PlayableTrack, RepeatMode } from './types';
import { createEngine } from './engine';
import { EventSourceImpl } from './eventSource';
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
 * Build a player track from a song at the chosen quality. Async: the stream
 * URL is a short-lived signed URL minted per track when (re)building the queue.
 * A downloaded track plays from its local file instead (offline, quality-setting
 * doesn't apply); artwork always points at the server since covers aren't downloaded.
 */
async function songToTrack(client: ImmerleClient, song: Song, qualityId: string): Promise<PlayableTrack> {
  // Unresolved (federated-playlist) entries have no playable id yet — resolve
  // via TrackList's tap flow first. Return an empty-url placeholder instead of
  // throwing, so one bad entry doesn't break building the whole queue (skipped
  // over on landing — see the trackChange handler below).
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
  /** Playlist the current queue was loaded from, if any — lets an unresolved
   * federated track hit mid-playback be resolved by position (see
   * resolveAndPlayUnresolved) instead of just skipped. */
  playlistId: string | null;
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
  /** Whether castTargetId is connected right now (see PlayQueueSnapshot.targetOnline). */
  castTargetOnline: boolean;
  /**
   * The device id that last wrote the mirrored state shown here (see
   * applyDisplaySnapshot) — who's actually making the sound right now, even
   * in unrestricted mode where castTargetId is empty. '' when unknown (e.g.
   * this device itself is the source, or nothing's ever been saved).
   */
  playingDeviceId: string;
  /**
   * Whether the last few cross-device sync attempts (SSE/poll) reached the
   * server. Never gates local playback — a downloaded track keeps playing
   * regardless (see songToTrack/offlinePlayableUrl) — this is purely an
   * informational signal for a small "offline" indicator.
   */
  serverReachable: boolean;

  init: () => Promise<void>;
  hydrateSettings: () => Promise<void>;

  playSongs: (songs: Song[], startIndex?: number, playlistId?: string) => Promise<void>;
  /**
   * Plays songs with shuffle mode turned on, instead of each screen
   * (liked/album/artist) hand-rolling its own one-off shuffled array — that
   * left the shuffle toggle in the player bar/full player out of sync with
   * what was actually playing. Loads in original order (so orderBackup
   * captures the true original for turning shuffle back off later), then
   * enables shuffle through the normal toggle if it wasn't already on.
   */
  playShuffled: (songs: Song[]) => Promise<void>;
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
  /** Make `deviceId` the sole active player (Spotify Connect's "Play on"). */
  setCastTarget: (deviceId: string) => Promise<void>;

  current: () => Song | undefined;
}

// Module-scoped scrobble state (not reactive UI state).
let scrobble: ScrobbleFlags = { nowPlayingSent: false, submitted: false };
// See scheduleSaveQueue/flushSaveQueue.
let lastQueueSaveAt = 0;
const QUEUE_SAVE_THROTTLE_MS = 5000;
// Highest remote.commandSeq this device has already considered while active
// (see reconcilePlayQueue) — distinguishes a new spectator command from one
// already applied/ignored. Reseeded whenever this device (re)becomes active
// (restoreQueue, or reconcilePlayQueue's takeover branch) so stale commands
// from before that point are never (re)applied.
let lastAppliedCommandSeq = 0;
// Original queue order, kept so shuffle can be turned off again.
let orderBackup: Song[] | null = null;
// True while an engine reload (setQueue) is mid-flight — a fresh load
// momentarily reports the *previous* track's position/status before the
// explicit reset takes effect. Without this, a stray event in that gap gets
// mistaken for this device's real position, starting a new track from
// wherever the old one had gotten to.
let suppressEngineEvents = false;
// How long to keep suppressing past the reload settling. Generous on
// purpose: a remote/on-demand track's old source can take a while to abort
// over a real network, and missing a genuine late update briefly is far
// cheaper than a stray event silently corrupting the saved position.
const ENGINE_RELOAD_GRACE_MS = 3000;
// Whether the local engine holds the queue currently shown. False once this
// device only mirrors another one (applyDisplaySnapshot): a local action then
// reloads the shared queue first (takeControl) instead of resuming whatever
// stale queue/position the idle engine still had.
let engineLoaded = false;
// Set by claimActiveDevice: until this time, a snapshot naming another
// device as target predates our claim (it was in flight while the claim was)
// and must not make this device pause itself and fall back to spectating.
let claimingUntil = 0;
const CLAIM_GRACE_MS = 5000;
// Every play-queue write from this device (claim, saves, commands) goes
// through this chain so they reach the server in the order they were made:
// a save racing ahead of its own claim would be rejected as coming from a
// non-active device (409), a late save could overwrite a newer one.
let writeChain: Promise<unknown> = Promise.resolve();
function queueWrite(write: () => Promise<unknown>): void {
  writeChain = writeChain.then(write).catch(() => undefined);
}
// Snapshots are reconciled one at a time, in arrival order: two overlapping
// reconciles (a takeover still loading while a command lands) would otherwise
// interleave their engine calls.
let reconcileChain: Promise<void> = Promise.resolve();
// changedAt of the state a remote currently shows (see applyDisplaySnapshot).
let shownChangedAt = 0;
// Set by sendRemoteCommand: until the active device saves a state newer than
// what was shown when the command left (or this deadline passes, for a
// command that turned out to be a no-op), incoming snapshots still describe
// the pre-command state. The command itself triggers one right away; showing
// it would undo the optimistic update (pause, then the icon flips back to
// playing until the real state lands).
let awaitingCommandUntil = 0;
let awaitingNewerThan = 0;
const COMMAND_ACK_TIMEOUT_MS = 3000;

// Chained promise backing acquireEngineReloadLock — always the completion
// promise of whichever reload most recently claimed the lock (already
// resolved if none is in flight).
let engineReloadChain: Promise<void> = Promise.resolve();
// Bumped by every acquireEngineReloadLock() call — lets a caller that was
// queued behind another one notice, once it's finally its turn, that a
// *newer* reload was requested in the meantime (see stale() below).
let reloadSeq = 0;

/**
 * Exclusive lock for reloading the engine (setQueue + friends), serialized
 * against any other in-flight reload. Call as the very first statement,
 * before any `await` — capturing/replacing `engineReloadChain` happens
 * synchronously, so two concurrent callers deterministically chain rather
 * than both slipping through (an earlier `while (suppressEngineEvents) await
 * sleep()` version had a gap that let two reloads run concurrently and mix
 * one's track with another's position/status).
 *
 * Serializing isn't enough alone: a reload queued behind another must only
 * still run if it's still the *latest* request, or it replays a decision
 * that's since been superseded (e.g. a takeover reconciliation queued behind
 * the user's own more recent play action would otherwise steamroll it once
 * its turn came). Callers must check `stale()` right after acquiring and
 * bail out if true.
 *
 * Sets suppressEngineEvents for the reload's duration; release() keeps it
 * suppressed for one more grace period after — see ENGINE_RELOAD_GRACE_MS.
 */
function acquireEngineReloadLock(): Promise<{ stale: () => boolean; release: () => void }> {
  const mySeq = ++reloadSeq;
  const previous = engineReloadChain;
  let releaseChain = () => {};
  engineReloadChain = new Promise<void>((resolve) => {
    releaseChain = resolve;
  });
  return previous.then(() => {
    suppressEngineEvents = true;
    return {
      stale: () => mySeq !== reloadSeq,
      release: () => {
        setTimeout(() => {
          suppressEngineEvents = false;
          releaseChain();
        }, ENGINE_RELOAD_GRACE_MS);
      },
    };
  });
}

/**
 * Waits for any in-flight engine reload to finish without becoming a lock
 * holder itself — for playNext/enqueue, which touch the engine (add/move)
 * but don't reload it wholesale, so they don't need their own grace period.
 */
function waitForEngineReloadSlot(): Promise<void> {
  return engineReloadChain;
}

function client(): ImmerleClient | null {
  return useAuth.getState().client;
}

// Consecutive-failure tracking behind the "server unreachable" badge (see
// AudioState.serverReachable) — flips off after a couple of consecutive
// failures of the live-sync channel (poll or SSE), avoiding flapping on one
// dropped request; flips back on any success. Purely informational: local
// (downloaded-track) playback never depends on this.
let consecutiveSyncFailures = 0;
const UNREACHABLE_AFTER_FAILURES = 2;
// For SSE: how long to go without an open/message before treating the
// connection as down — 2x the server's own 20s heartbeat (see
// handleStreamPlayQueue), so one missed beat isn't a false positive.
const SSE_SILENCE_TIMEOUT_MS = 45000;

function noteSyncResult(set: (partial: Partial<AudioState>) => void, ok: boolean): void {
  if (ok) {
    consecutiveSyncFailures = 0;
    set({ serverReachable: true });
    return;
  }
  consecutiveSyncFailures += 1;
  if (consecutiveSyncFailures >= UNREACHABLE_AFTER_FAILURES) set({ serverReachable: false });
}

/**
 * Whether this device is remote-controlling another one that's actually
 * there (Spotify Connect's "Playing on X"). An offline target doesn't count:
 * nobody would ever apply our commands, so actions run locally instead.
 */
function isSpectating(get: () => AudioState): boolean {
  const myId = client()?.getSession()?.deviceId;
  const { castTargetId: target, castTargetOnline } = get();
  return !!target && target !== myId && castTargetOnline;
}

/**
 * Claim the active-device role before driving the local engine: every
 * action touching `engine` (playSongs, enqueue, a local toggle, ...) must
 * call this first. Like Spotify Connect there's always one active device:
 * playing here moves playback here, and every other device pauses and turns
 * into a remote for this one. Fire-and-forget (but ordered before this
 * device's next save, see queueWrite): the local playback about to start is
 * the source of truth regardless of whether the claim has landed yet.
 */
function claimActiveDevice(get: () => AudioState, set: (partial: Partial<AudioState>) => void): void {
  const c = client();
  const myId = c?.getSession()?.deviceId;
  if (!c || !myId || get().castTargetId === myId) return;
  claimingUntil = Date.now() + CLAIM_GRACE_MS;
  set({ castTargetId: myId, castTargetOnline: true, playingDeviceId: myId });
  queueWrite(() => c.setPlaybackTarget(myId));
}

/**
 * For a transport action (toggle/next/seek/...) about to run locally (not
 * spectating): claim the active role and, if the engine only mirrors another
 * device's queue, load the shared queue at its saved position first: so
 * pressing play after the active device went away resumes right where it
 * stopped, here.
 */
async function takeControl(get: () => AudioState, set: (partial: Partial<AudioState>) => void): Promise<void> {
  claimActiveDevice(get, set);
  if (engineLoaded) return;
  const remote = await client()?.getPlayQueue().catch(() => null);
  if (remote) await applyRemoteQueue(get, set, remote, false);
}

/**
 * Send a remote-control command to the server — how a spectator (see
 * isSpectating) controls the active device, which applies it via its own
 * next()/toggle()/seekTo()/etc. (see reconcilePlayQueue) rather than
 * adopting a computed snapshot wholesale. Fire-and-forget: no ack/timeout.
 */
function sendRemoteCommand(get: () => AudioState, cmd: Omit<PlayQueueCommand, 'forTarget' | 'issuedBy'>): void {
  const c = client();
  if (!c) return;
  const forTarget = get().castTargetId;
  const issuedBy = c.getSession()?.deviceId;
  // eslint-disable-next-line no-console
  console.log('[playqueue] command', { ...cmd, forTarget, issuedBy });
  awaitingNewerThan = shownChangedAt;
  awaitingCommandUntil = Date.now() + COMMAND_ACK_TIMEOUT_MS;
  queueWrite(() => c.sendPlayQueueCommand({ ...cmd, forTarget, issuedBy }));
}

/**
 * Load a saved server-side queue into local state and the engine, at its
 * saved position — shared by the launch restore and by "this device is (or
 * just became) the active player". Lands paused unless `autoplay`.
 *
 * `verifyStillCurrent` (reconcilePlayQueue's takeover path only): re-fetch
 * the queue after acquiring the reload lock and bail if changedBy has moved
 * on from `remote.changedBy`. The reload-lock only orders by *call* order,
 * not data freshness — an SSE-sourced takeover can queue up after the
 * user's own more recent play action (so it isn't stale by call order) while
 * still carrying older data, and would silently steamroll it. Re-checking
 * server state right before committing catches this regardless of order.
 */
async function applyRemoteQueue(
  get: () => AudioState,
  set: (partial: Partial<AudioState>) => void,
  remote: PlayQueueSnapshot,
  autoplay: boolean,
  verifyStillCurrent = false,
): Promise<void> {
  const { stale, release } = await acquireEngineReloadLock();
  try {
    if (stale()) return; // superseded by a newer local/remote reload while queued — see acquireEngineReloadLock
    const c = client();
    const engine = get().engine;
    if (!c || !engine || !remote.songs.length) return;
    if (verifyStillCurrent) {
      const fresh = await c.getPlayQueue().catch(() => null);
      if (fresh && fresh.changedBy !== remote.changedBy) return; // someone (possibly this device) has already written something newer
    }
    const idx = Math.max(0, remote.songs.findIndex((s) => s.id === remote.currentId));
    orderBackup = null;
    scrobble = { nowPlayingSent: false, submitted: false };
    set({
      songs: remote.songs,
      playlistId: null, // not tracked across devices — an unresolved track here just skips, see resolveAndPlayUnresolved
      index: idx,
      position: remote.positionMs / 1000,
      duration: remote.songs[idx]?.duration ?? 0,
      playingDeviceId: client()?.getSession()?.deviceId ?? '',
      // Adopt the saved shuffle/repeat mode — this device is becoming (or
      // remains) the one actually driving playback, so its own transport
      // mode should match what was last saved, not its prior local value.
      shuffle: remote.shuffle,
      repeat: remote.repeat,
    });
    const tracks = await Promise.all(remote.songs.map((s) => songToTrack(c, s, get().qualityId)));
    // setQueue only loads (paused) — seek before playing, so a fresh source
    // never briefly plays from 0 and races the seek (see engine.web.ts).
    await engine.setQueue(tracks, idx);
    engineLoaded = true;
    await engine.seekTo(remote.positionMs / 1000);
    await engine.setRepeatMode(remote.repeat);
    if (autoplay) await engine.play();
  } finally {
    // Grace period past settling: a browser audio element's pause/playing/
    // waiting events don't fire in lockstep with the promises above resolving,
    // so a trailing one landing as the guard lifts would otherwise override
    // the reassertion below with a stale mid-transition value.
    release();
  }
  // Re-assert: the engine's own events were dropped (not corrected) during
  // the guarded window, so status would otherwise stay whatever it was before.
  set({ position: remote.positionMs / 1000, status: autoplay ? 'playing' : 'paused' });
  sendNowPlaying(get);
  // Forced past isSpectating: this device is reclaiming the shared queue, so
  // a stale write from whoever it took over from doesn't keep getting
  // re-applied on every poll.
  flushSaveQueue(get, true, 'applyRemoteQueue');
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
  if (Date.now() < awaitingCommandUntil && (remote.changedAt ?? 0) <= awaitingNewerThan) return; // pre-command state, see awaitingCommandUntil
  awaitingCommandUntil = 0;
  shownChangedAt = remote.changedAt ?? 0;
  engineLoaded = false; // the shown queue is no longer what the engine holds: see takeControl
  const idx = Math.max(0, remote.songs.findIndex((s) => s.id === remote.currentId));
  const duration = remote.songs[idx]?.duration ?? 0;
  // A target that went away isn't playing anymore, whatever it last saved.
  const playing = remote.playing && (!remote.targetDeviceId || remote.targetOnline);
  // The saved position is as of changedAt (up to a save interval ago):
  // extrapolate so the bar shows where the music actually is.
  let position = remote.positionMs / 1000;
  if (playing && remote.changedAt) position += Math.max(0, (Date.now() - remote.changedAt) / 1000);
  if (duration > 0) position = Math.min(position, duration);
  set({
    songs: remote.songs,
    index: idx,
    position,
    duration,
    status: playing ? 'playing' : 'paused',
    // Prefer the explicit cast target (who's supposed to be playing) over
    // changedBy (who last wrote) — the target stays authoritative even for
    // one event cycle where a fresh write hasn't landed from them yet.
    playingDeviceId: remote.targetDeviceId ? (remote.targetOnline ? remote.targetDeviceId : '') : playing ? remote.changedBy || '' : '',
    // Mirror the actual active device's shuffle/repeat mode rather than
    // leaving this device's own (possibly stale) local value shown — see
    // models.PlayQueue.Shuffle/Repeat.
    shuffle: remote.shuffle,
    repeat: remote.repeat,
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
  set({ castTargetId: remote.targetDeviceId, castTargetOnline: remote.targetOnline });
  // Whatever command happens to be sitting on the queue at launch predates
  // this device even being here — never a fresh instruction. See
  // lastAppliedCommandSeq.
  lastAppliedCommandSeq = remote.commandSeq;
  const myId = c.getSession()?.deviceId;
  if (remote.targetDeviceId && remote.targetDeviceId !== myId) {
    applyDisplaySnapshot(set, remote);
  } else {
    await applyRemoteQueue(get, set, remote, false);
  }
}

/**
 * Where a command's track (trackId) sits in this device's own queue, or -1 if
 * it isn't there (nothing safe to do). queueIndex only picks the nearest of
 * duplicates, never the primary lookup: the sender's queue can differ.
 */
function resolveQueueIndex(get: () => AudioState, cmd: PlayQueueCommand): number {
  if (!cmd.trackId) return -1;
  const matches: number[] = [];
  get().songs.forEach((s, i) => {
    if (s.id === cmd.trackId) matches.push(i);
  });
  if (matches.length === 0) return -1;
  const hint = cmd.queueIndex;
  if (matches.length === 1 || hint == null) return matches[0];
  return matches.reduce((best, i) => (Math.abs(i - hint) < Math.abs(best - hint) ? i : best));
}

/**
 * Applies a spectator's command via this device's own action methods, as if
 * tapped locally — a command is an instruction to *do something*, never a
 * snapshot to adopt. skipTo resolves trackId against this device's own queue
 * (not the sender's, which can momentarily differ); queueIndex only
 * disambiguates a duplicate trackId, never used as the primary lookup.
 */
async function applyCommand(get: () => AudioState, cmd: PlayQueueCommand): Promise<void> {
  switch (cmd.type) {
    case 'toggle':
      await get().toggle();
      return;
    case 'next':
      await get().next();
      return;
    case 'previous':
      await get().previous();
      return;
    case 'seekTo':
      if (cmd.positionMs != null) await get().seekTo(cmd.positionMs / 1000);
      return;
    case 'skipTo': {
      const idx = resolveQueueIndex(get, cmd);
      if (idx >= 0) await get().skipTo(idx);
      return;
    }
    case 'removeAt': {
      const idx = resolveQueueIndex(get, cmd);
      if (idx >= 0) await get().removeAt(idx);
      return;
    }
    case 'move': {
      const idx = resolveQueueIndex(get, cmd);
      if (idx >= 0 && cmd.toIndex != null) await get().move(idx, Math.min(cmd.toIndex, get().songs.length - 1));
      return;
    }
    case 'toggleShuffle':
      await get().toggleShuffle();
      return;
    case 'cycleRepeat':
      await get().cycleRepeat();
      return;
    case 'playNext':
    case 'enqueue': {
      const c = client();
      if (!c || !cmd.trackIds?.length) return;
      const songs = (await Promise.all(cmd.trackIds.map((id) => c.getSong(id).catch(() => null)))).filter((s): s is Song => !!s);
      if (songs.length) await (cmd.type === 'playNext' ? get().playNext(songs) : get().enqueue(songs));
      return;
    }
  }
}

/**
 * Reconciles this device against a freshly-received queue snapshot (SSE
 * stream, see connectPlayQueueLive).
 *
 * The device explicitly targeted as active is the source of truth for its
 * own playback: once it's already been the target (wasTarget), it never
 * adopts an ordinary broadcast, only an explicit command addressed to its
 * tenure (see applyCommand). The exception is the moment it *becomes* the
 * target: a one-time takeover of the outgoing device's session (see
 * applyRemoteQueue).
 *
 * Every other device only mirrors state for display (applyDisplaySnapshot)
 * and never starts local audio on its own. A device with no target that's
 * already playing is left alone too — self-authoritative for its own
 * independent session.
 */
async function reconcilePlayQueue(
  get: () => AudioState,
  set: (partial: Partial<AudioState>) => void,
  remote: PlayQueueSnapshot,
): Promise<void> {
  const engine = get().engine;
  const myId = client()?.getSession()?.deviceId;
  const target = remote.targetDeviceId;
  const wasTarget = get().castTargetId === myId; // read before the set() below
  // eslint-disable-next-line no-console
  console.log('[playqueue] reconcile', {
    myId,
    target,
    wasTarget,
    changedBy: remote.changedBy,
    current: remote.currentId,
    playing: remote.playing,
    commandSeq: remote.commandSeq,
    hasEngine: !!engine,
  });
  if (!engine) return;
  if (target !== myId && Date.now() < claimingUntil) return; // predates our own claim: see claimingUntil
  if (target === myId) claimingUntil = 0;
  set({ castTargetId: target, castTargetOnline: remote.targetOnline });

  if (target && target === myId) {
    if (!wasTarget) {
      // eslint-disable-next-line no-console
      console.log('[playqueue] taking over as active device');
      lastAppliedCommandSeq = remote.commandSeq; // starting a fresh tenure — see lastAppliedCommandSeq
      await applyRemoteQueue(get, set, remote, remote.playing, true);
      return;
    }
    // Already the active device: never adopt an ordinary broadcast — only an
    // explicit command addressed to this tenure.
    if (remote.commandSeq > lastAppliedCommandSeq) {
      lastAppliedCommandSeq = remote.commandSeq;
      if (remote.pendingCommand && remote.pendingCommand.forTarget === myId) {
        // eslint-disable-next-line no-console
        console.log('[playqueue] applying command', remote.pendingCommand);
        await applyCommand(get, remote.pendingCommand);
      }
    }
    return;
  }

  if (target) {
    // Handed off elsewhere: stop the local audio (avoid double audio). Not
    // get().status: it may already mirror the remote "playing" state.
    if (engineLoaded) await engine.pause();
    applyDisplaySnapshot(set, remote);
    return;
  }

  // Unrestricted mode: a device already playing its own independent session
  // is self-authoritative too — an unrelated device's broadcast must not
  // overwrite what's showing here (applyDisplaySnapshot never touches the
  // engine, but the displayed track/position would flip otherwise).
  if (get().status === 'playing') return;
  applyDisplaySnapshot(set, remote);
}

/**
 * Live-updates this device on every play-queue change over SSE (the
 * browser's EventSource on web, an expo/fetch-based one on native: see
 * eventSource.native.ts). The open stream is also this device's presence:
 * spectators see the active device go offline the moment it closes. See
 * reconcilePlayQueue. Both implementations reconnect on their own.
 */
function connectPlayQueueLive(get: () => AudioState, set: (partial: Partial<AudioState>) => void): void {
  const c = client();
  if (!c) return;
  const ES = EventSourceImpl;
  if (ES) {
    const url = c.playQueueEventsUrl();
    // eslint-disable-next-line no-console
    console.log('[playqueue] connecting SSE', url.replace(/apiKey=[^&]+/, 'apiKey=***'));
    const es = new ES(url);
    let silenceTimer: ReturnType<typeof setTimeout> | null = null;
    const resetSilenceTimer = () => {
      if (silenceTimer) clearTimeout(silenceTimer);
      silenceTimer = setTimeout(() => noteSyncResult(set, false), SSE_SILENCE_TIMEOUT_MS);
    };
    es.addEventListener('open', () => {
      // eslint-disable-next-line no-console
      console.log('[playqueue] SSE open');
      noteSyncResult(set, true);
      resetSilenceTimer();
    });
    es.addEventListener('error', (e) => {
      // eslint-disable-next-line no-console
      console.warn('[playqueue] SSE error (browser will auto-reconnect)', e);
      // Don't flip unreachable immediately — EventSource auto-reconnects;
      // only declare it down if that hasn't succeeded within the timeout.
      resetSilenceTimer();
    });
    es.addEventListener('state', (e: { data?: string }) => {
      // eslint-disable-next-line no-console
      console.log('[playqueue] SSE message', e.data);
      noteSyncResult(set, true);
      resetSilenceTimer();
      if (!e.data) return;
      try {
        const remote = toPlayQueueSnapshot(JSON.parse(e.data) as PlayQueueView);
        reconcileChain = reconcileChain.then(() => reconcilePlayQueue(get, set, remote)).catch(() => undefined);
      } catch (err) {
        // eslint-disable-next-line no-console
        console.warn('[playqueue] failed to parse SSE event', err);
      }
    });
    // Jam invites ride this same connection instead of a dedicated stream —
    // see internal/api/immerle/playqueue.go's handleStreamPlayQueue for why.
    // useJamInvites (query/social.ts) reads this cache entry; on native (no
    // EventSource) it falls back to its own polling instead.
    es.addEventListener('invites', (e: { data?: string }) => {
      if (!e.data) return;
      try {
        const parsed = JSON.parse(e.data) as { invites?: JamInviteDTO[] };
        queryClient.setQueryData(['jam', 'invites', 'mine'], (parsed.invites ?? []).map(toJamInvite));
      } catch {
        /* ignore malformed events */
      }
    });
    resetSilenceTimer();
  }
}

/**
 * While spectating, position only moves in jumps (once per SSE push / poll
 * interval), reading as the progress bar visibly skipping. Tick it forward
 * locally once a second between updates; every real update
 * (applyDisplaySnapshot) still overwrites it, so drift stays under ~2s.
 */
function startFakeProgressTicker(get: () => AudioState, set: (partial: Partial<AudioState>) => void): void {
  setInterval(() => {
    if (!isSpectating(get) || get().status !== 'playing') return;
    const { position, duration } = get();
    set({ position: duration > 0 ? Math.min(position + 1, duration) : position + 1 });
  }, 1000);
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

/**
 * A federated-playlist track hit mid-playback while still unresolved (empty-url
 * placeholder — see songToTrack): resolve it by its playlist position, same as
 * TrackList's tap-to-resolve flow (playlist/[id].tsx's playUnresolved), then
 * swap it into the running queue in place so playback continues instead of
 * skipping. Falls back to the skip+toast if there's no playlist context (a
 * queue not sourced from a playlist) or resolution fails (unresolvable track).
 */
async function resolveAndPlayUnresolved(
  get: () => AudioState,
  set: (partial: Partial<AudioState>) => void,
  index: number,
  song: Song,
): Promise<void> {
  const c = client();
  const engine = get().engine;
  const playlistId = get().playlistId;
  if (c && engine && playlistId) {
    try {
      const resolved = await c.resolvePlaylistTrack(playlistId, index);
      if (get().index === index && get().songs[index]?.unresolved) {
        const songs = [...get().songs];
        songs[index] = resolved;
        set({ songs });
        await engine.replaceAt(index, await songToTrack(c, resolved, get().qualityId));
        // The placeholder never actually played (empty url), so replaceAt's own
        // wasPlaying check (engine.web.ts) sees status stuck at 'loading' and
        // won't resume, so force it explicitly, same as the manual tap-resolve
        // flow (playlist/[id].tsx's playUnresolved -> playSongs) does.
        await engine.play();
        return;
      }
    } catch {
      /* fall through to skip+toast below */
    }
  }
  useToast.getState().warning(t('media.player.unresolvedSkipped', { title: song.title }));
  void get().engine?.next();
}

export const usePlayer = create<AudioState>((set, get) => ({
  engine: null,
  songs: [],
  playlistId: null,
  index: -1,
  status: 'idle',
  position: 0,
  duration: 0,
  repeat: 'off',
  shuffle: false,
  volume: 1,
  qualityId: DEFAULT_QUALITY_ID,
  castTargetId: '',
  castTargetOnline: false,
  playingDeviceId: '',
  serverReachable: true,

  hydrateSettings: async () => {
    try {
      const q = await AsyncStorage.getItem(QUALITY_KEY);
      if (q) set({ qualityId: q });
      const v = await AsyncStorage.getItem(VOLUME_KEY);
      if (v !== null) {
        const volume = Math.max(0, Math.min(1, Number(v)));
        set({ volume });
        // init() may have created the engine before this read resolved: push
        // the saved volume to it too, or it keeps playing at 1.0.
        void get().engine?.setVolume(volume);
      }
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

    // While spectating, the local engine sits idle (applyDisplaySnapshot
    // never touches it) but can still fire a stray state/progress event on
    // the way to idle — without this guard it would win the race and stomp
    // the mirrored remote status/position with this device's own idle values.
    engine.on('state', (s) => {
      if (isSpectating(get) || suppressEngineEvents) return;
      const wasPlaying = get().status === 'playing';
      set({ status: s.status, index: s.index, duration: s.duration || get().duration });
      // Not throttled: a pause could sit unsaved indefinitely otherwise
      // (progress ticks, the other save trigger, stop once paused); a resume
      // would leave a spectator's UI stuck on "paused" for up to
      // QUEUE_SAVE_THROTTLE_MS.
      if (wasPlaying !== (s.status === 'playing')) flushSaveQueue(get, false, 'engine:state-change');
    });
    engine.on('progress', (position, duration) => {
      if (isSpectating(get) || suppressEngineEvents) return;
      set({ position, duration: duration || get().duration });
      maybeScrobble(get, position, duration);
      scheduleSaveQueue(get);
    });
    engine.on('trackChange', (index) => {
      if (isSpectating(get) || suppressEngineEvents) return;
      scrobble = { nowPlayingSent: false, submitted: false };
      set({ index, position: 0 });
      sendNowPlaying(get);
      flushSaveQueue(get, false, 'engine:trackChange');
      // Unresolved federated-playlist track (empty-url placeholder — see
      // songToTrack): resolve it in place instead of sitting on dead air.
      // ponytail: if the whole queue is unresolved this cascades resolve calls
      // to the end (or forever, with repeat on) — fine for the size of
      // federated playlists today.
      const song = get().songs[index];
      if (song?.unresolved) void resolveAndPlayUnresolved(get, set, index, song);
    });

    // Publish the engine before applying the volume, so a hydrateSettings()
    // resolving meanwhile either sees it (and applies its volume) or has
    // already updated get().volume read below.
    set({ engine });
    await engine.setVolume(get().volume);
    startFakeProgressTicker(get, set);

    // Cross-device state: show what's playing on launch, then stay
    // live-updated on any device's changes (see connectPlayQueueLive).
    // init() races useAuth's restore() — client() is usually still null here
    // since session restore is async. restoreQueue/connectPlayQueueLive
    // silently no-op without a client and nothing else would retry them, so
    // wait for the client to actually exist first.
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
    // native's app-background lifecycle has no equivalent moment). Catches
    // state that hasn't hit the 5s throttle yet.
    if (typeof document !== 'undefined' && typeof document.addEventListener === 'function') {
      document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') flushSaveQueue(get, false, 'visibilitychange');
      });
    }
  },

  playSongs: async (songs, startIndex = 0, playlistId) => {
    const { stale, release } = await acquireEngineReloadLock();
    try {
      if (stale()) return; // superseded while queued — see acquireEngineReloadLock
      const c = client();
      const engine = get().engine;
      if (!c || !engine || songs.length === 0) return;
      claimActiveDevice(get, set);
      const tracks = await Promise.all(songs.map((s) => songToTrack(c, s, get().qualityId)));
      orderBackup = null; // new playback context invalidates any shuffle backup
      set({ songs, index: startIndex, position: 0, duration: songs[startIndex]?.duration ?? 0, playlistId: playlistId ?? null });
      scrobble = { nowPlayingSent: false, submitted: false };
      await engine.setQueue(tracks, startIndex); // loads paused — see engine.setQueue
      engineLoaded = true;
      await engine.play();
    } finally {
      release();
    }
    set({ position: 0, status: 'playing' });
    sendNowPlaying(get);
    // Forced: claimActiveDevice's server write may still be in flight, and
    // the device being taken over from can land an ambient broadcast
    // carrying the *old* target in the meantime, making isSpectating look
    // true again right as this save runs — force bypasses that.
    flushSaveQueue(get, true, 'playSongs');
  },

  playShuffled: async (songs) => {
    if (songs.length === 0) return;
    await get().playSongs(songs, 0);
    if (!get().shuffle) await get().toggleShuffle();
  },

  playRadio: async (station) => {
    const { stale, release } = await acquireEngineReloadLock();
    try {
      if (stale()) return; // superseded while queued — see acquireEngineReloadLock
      const engine = get().engine;
      if (!engine || !station.streamUrl) return;
      claimActiveDevice(get, set);
      // Live streams aren't scrobbled and have no real duration. The raw URL
      // is played directly (not routed through the Subsonic stream endpoint).
      const track: PlayableTrack = { id: station.id, url: station.streamUrl, title: station.name, artist: '', duration: 0 };
      const c = client();
      const coverUrl = station.hasCover && c ? c.radioCoverUrl(station.id) : undefined;
      const song = { id: station.id, title: station.name, artist: '', coverUrl } as Song;
      orderBackup = null;
      scrobble = { nowPlayingSent: true, submitted: true };
      set({ songs: [song], index: 0, position: 0, playlistId: null });
      await engine.setQueue([track], 0); // loads paused — see engine.setQueue
      engineLoaded = true;
      await engine.play();
    } finally {
      release();
    }
    set({ position: 0, status: 'playing' });
    flushSaveQueue(get, true, 'playRadio'); // see playSongs — claimActiveDevice's write may not have landed yet
  },

  playTrackById: async (id, positionSec, autoplay) => {
    const { stale, release } = await acquireEngineReloadLock();
    try {
      if (stale()) return; // superseded while queued — see acquireEngineReloadLock
      const c = client();
      const engine = get().engine;
      if (!c || !engine) return;
      claimActiveDevice(get, set);
      const song = await c.getSong(id).catch(() => ({ id, title: 'Piste' }) as Song);
      orderBackup = null;
      scrobble = { nowPlayingSent: false, submitted: false };
      set({ songs: [song], index: 0, position: positionSec, duration: song.duration ?? 0, playlistId: null });
      // setQueue only loads (paused) — seek before playing, see applyRemoteQueue.
      await engine.setQueue([await songToTrack(c, song, get().qualityId)], 0);
      engineLoaded = true;
      await engine.seekTo(positionSec);
      if (autoplay) await engine.play();
    } finally {
      release();
    }
    set({ position: positionSec, status: autoplay ? 'playing' : 'paused' });
    sendNowPlaying(get);
    flushSaveQueue(get, true, 'playTrackById'); // see playSongs — claimActiveDevice's write may not have landed yet
  },

  playNext: async (songs) => {
    await waitForEngineReloadSlot();
    const c = client();
    const engine = get().engine;
    if (!c || !engine) return get().playSongs(songs);
    if (get().songs.length === 0) return get().playSongs(songs);
    if (isSpectating(get)) {
      // Like Spotify Connect: queue it on the active device, playback stays there.
      const at = get().index + 1;
      const next = [...get().songs];
      next.splice(at, 0, ...songs);
      set({ songs: next }); // optimistic
      sendRemoteCommand(get, { type: 'playNext', trackIds: songs.map((s) => s.id) });
      return;
    }
    await takeControl(get, set);
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
    flushSaveQueue(get, false, 'playNext');
  },

  enqueue: async (songs) => {
    await waitForEngineReloadSlot();
    const c = client();
    const engine = get().engine;
    if (!c || !engine) return;
    if (get().songs.length === 0) return get().playSongs(songs);
    if (isSpectating(get)) {
      set({ songs: [...get().songs, ...songs] }); // optimistic: see playNext
      sendRemoteCommand(get, { type: 'enqueue', trackIds: songs.map((s) => s.id) });
      return;
    }
    await takeControl(get, set);
    set({ songs: [...get().songs, ...songs] });
    await engine.add(await Promise.all(songs.map((s) => songToTrack(c, s, get().qualityId))));
    flushSaveQueue(get, false, 'enqueue');
  },

  toggle: async () => {
    if (isSpectating(get)) {
      const playing = get().status !== 'playing';
      set({ status: playing ? 'playing' : 'paused' }); // optimistic; the active device converges once it applies the command
      sendRemoteCommand(get, { type: 'toggle' });
      return;
    }
    const engine = get().engine;
    if (!engine) return;
    await takeControl(get, set);
    const wasPlaying = get().status === 'playing';
    // Optimistic: right after a reload, the engine's real events are still
    // swallowed by the grace window (suppressEngineEvents), which would
    // otherwise leave the button stuck on "play" while audio plays. A later
    // 'state' event corrects this if the actual outcome differs.
    set({ status: wasPlaying ? 'paused' : 'playing' });
    if (wasPlaying) await engine.pause();
    else await engine.play();
    // Immediately: the optimistic set above means the engine's own 'state'
    // event sees no transition and wouldn't save, leaving remotes on the old
    // state until the next progress save.
    flushSaveQueue(get, false, 'toggle');
  },

  next: async () => {
    if (isSpectating(get)) {
      const { songs, index } = get();
      const song = songs[index + 1];
      if (!song) return; // nothing to skip to per our own mirrored queue
      set({ index: index + 1, position: 0, status: 'playing' }); // optimistic
      sendRemoteCommand(get, { type: 'next' });
      return;
    }
    await takeControl(get, set);
    await get().engine?.next();
  },

  previous: async () => {
    if (isSpectating(get)) {
      const { songs, index } = get();
      const song = songs[index - 1];
      if (!song) return;
      set({ index: index - 1, position: 0, status: 'playing' }); // optimistic
      sendRemoteCommand(get, { type: 'previous' });
      return;
    }
    await takeControl(get, set);
    await get().engine?.previous();
    // Immediately, not throttled: restarting the current track (the >3s-in
    // case, see engine.previous) emits no trackChange to flush from — see
    // engine:state-change/seekTo for the same reasoning.
    flushSaveQueue(get, false, 'previous');
  },

  seekTo: async (seconds) => {
    if (isSpectating(get)) {
      set({ position: seconds }); // optimistic
      sendRemoteCommand(get, { type: 'seekTo', positionMs: Math.floor(seconds * 1000) });
      return;
    }
    const engine = get().engine;
    if (!engine) return;
    await takeControl(get, set);
    // A not-yet-downloaded track streams progressively — no byte ranges, so a
    // seek would silently restart from 0. Swap in the now-local track if the
    // download finished, else bail with a toast. Guarded here (not just the
    // UI) so an OS media-session seek control can't trigger a raw seek.
    const index = get().index;
    const song = get().songs[index];
    if (song?.remote && !(await upgradeIfDownloaded(get, set, index, song))) {
      useToast.getState().warning(t('media.player.seekUnavailableRemote'));
      return;
    }
    await engine.seekTo(seconds);
    set({ position: seconds });
    // Immediately, not throttled — a seek is a discrete, deliberate change
    // (like a pause/resume, see engine:state-change) and a spectator
    // shouldn't wait out the progress-tick throttle to see it land.
    flushSaveQueue(get, false, 'seekTo');
  },

  skipTo: async (index) => {
    if (isSpectating(get)) {
      const song = get().songs[index];
      if (!song) return;
      set({ index, position: 0, status: 'playing' }); // optimistic
      sendRemoteCommand(get, { type: 'skipTo', trackId: song.id, queueIndex: index });
      return;
    }
    await takeControl(get, set);
    await get().engine?.skipTo(index);
  },

  removeAt: async (index) => {
    const engine = get().engine;
    if (!engine) return;
    if (isSpectating(get)) {
      // Like Spotify Connect: edit the active device's queue, playback stays there.
      const song = get().songs[index];
      if (!song || index === get().index) return; // removing what's playing elsewhere isn't a queue edit
      const songs = [...get().songs];
      songs.splice(index, 1);
      set({ songs, index: index < get().index ? get().index - 1 : get().index }); // optimistic
      sendRemoteCommand(get, { type: 'removeAt', trackId: song.id, queueIndex: index });
      return;
    }
    await takeControl(get, set);
    const songs = [...get().songs];
    songs.splice(index, 1);
    set({ songs });
    await engine.removeAt(index);
    flushSaveQueue(get, false, 'removeAt');
  },

  move: async (from, to) => {
    const engine = get().engine;
    if (!engine) return;
    if (isSpectating(get)) {
      const song = get().songs[from];
      if (!song) return;
      const songs = [...get().songs];
      songs.splice(from, 1);
      songs.splice(to, 0, song);
      const current = get().songs[get().index];
      set({ songs, index: current ? songs.indexOf(current) : get().index }); // optimistic, see removeAt
      sendRemoteCommand(get, { type: 'move', trackId: song.id, queueIndex: from, toIndex: to });
      return;
    }
    await takeControl(get, set);
    const songs = [...get().songs];
    const [item] = songs.splice(from, 1);
    if (item) songs.splice(to, 0, item);
    set({ songs });
    await engine.move(from, to);
    flushSaveQueue(get, false, 'move');
  },

  cycleRepeat: async () => {
    if (isSpectating(get)) {
      const order: RepeatMode[] = ['off', 'queue', 'track'];
      set({ repeat: order[(order.indexOf(get().repeat) + 1) % order.length] }); // optimistic
      sendRemoteCommand(get, { type: 'cycleRepeat' });
      return;
    }
    await takeControl(get, set);
    const order: RepeatMode[] = ['off', 'queue', 'track'];
    const next = order[(order.indexOf(get().repeat) + 1) % order.length];
    set({ repeat: next });
    await get().engine?.setRepeatMode(next);
    // Immediately, not throttled — like seekTo/pause, a deliberate discrete
    // change another device mirroring this queue should reflect right away
    // (see applyDisplaySnapshot/applyRemoteQueue).
    flushSaveQueue(get, false, 'cycleRepeat');
  },

  toggleShuffle: async () => {
    if (isSpectating(get)) {
      set({ shuffle: !get().shuffle }); // optimistic
      sendRemoteCommand(get, { type: 'toggleShuffle' });
      return;
    }
    await takeControl(get, set);
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

    const wasPlaying = get().status === 'playing';
    set({ shuffle: !shuffle, songs: nextSongs, index: nextIndex });
    if (nextSongs.length === 0) return;
    // Rebuild the engine queue around the (unchanged) current track, then
    // restore the playback position so the music doesn't visibly restart —
    // and only resume playing if it already was (setQueue loads paused).
    const tracks = await Promise.all(nextSongs.map((s) => songToTrack(c, s, get().qualityId)));
    await engine.setQueue(tracks, nextIndex);
    await engine.seekTo(position);
    if (wasPlaying) await engine.play();
    // Immediately, not throttled — see cycleRepeat.
    flushSaveQueue(get, false, 'toggleShuffle');
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
    const wasPlaying = get().status === 'playing';
    const tracks = await Promise.all(get().songs.map((s) => songToTrack(c, s, id)));
    await engine.setQueue(tracks, idx); // loads paused — see engine.setQueue
    await engine.seekTo(pos);
    if (wasPlaying) await engine.play();
  },

  setCastTarget: async (deviceId) => {
    const c = client();
    if (!c) return;
    try {
      await c.setPlaybackTarget(deviceId);
    } catch {
      return; // best-effort; UI keeps its previous state
    }
    set({ castTargetId: deviceId, castTargetOnline: true });
    if (!deviceId) return;
    const myId = c.getSession()?.deviceId;
    if (deviceId === myId) {
      const remote = await c.getPlayQueue().catch(() => null);
      if (remote) {
        lastAppliedCommandSeq = remote.commandSeq; // starting a fresh tenure — see lastAppliedCommandSeq
        await applyRemoteQueue(get, set, remote, true);
      }
      return;
    }
    if (engineLoaded) await get().engine?.pause();
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
 * writes while spectating: local songs/index/status are just a copy of what
 * the active device already told the server, so writing it back would churn
 * `changedBy` and trick the active device's poll into re-applying its own
 * command. sendRemoteCommand bypasses this — the one case a spectator
 * legitimately changes the shared session.
 *
 * `force` skips the isSpectating check — for playSongs/playRadio/
 * playTrackById's final save right after claimActiveDevice. That claim is a
 * fire-and-forget write; until it lands, an ambient broadcast from the
 * previously-active device can still reset castTargetId and make
 * isSpectating look true again, silently dropping the save meant to hand
 * this device the session. Once claimActiveDevice has run, this device's
 * intent is already decided locally, so the save must land regardless.
 */
function flushSaveQueue(get: () => AudioState, force = false, reason = 'unknown'): void {
  if (!force && isSpectating(get)) return;
  lastQueueSaveAt = Date.now();
  const c = client();
  const { songs, index, position, status, shuffle, repeat } = get();
  if (!c || songs.length === 0 || index < 0) return;
  // eslint-disable-next-line no-console
  console.log('[playqueue] flush', { reason, force, current: songs[index]?.id, position, status, shuffle, repeat, suppressEngineEvents });
  const current = songs[index]?.id;
  const playing = status === 'playing';
  queueWrite(() => c.savePlayQueue(songs, current, Math.floor(position * 1000), playing, shuffle, repeat));
}

/**
 * Throttled savePlayQueue for the high-frequency progress tick. A plain
 * debounce would never land during continuous playback (each tick cancels
 * the pending one), losing state entirely if the app closes mid-song. Save
 * at most once every 5s instead; discrete transitions call flushSaveQueue
 * directly so those are captured immediately.
 */
function scheduleSaveQueue(get: () => AudioState): void {
  if (Date.now() - lastQueueSaveAt < QUEUE_SAVE_THROTTLE_MS) return;
  flushSaveQueue(get, false, 'engine:progress-throttled');
}
