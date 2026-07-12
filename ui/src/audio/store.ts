import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import { useAuth } from '../auth/store';
import { queryClient } from '../query/queryClient';
import { ImmerleClient } from '../api/immerle/client';
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
let saveQueueTimer: ReturnType<typeof setTimeout> | null = null;
// Original queue order, kept so shuffle can be turned off again.
let orderBackup: Song[] | null = null;

function client(): ImmerleClient | null {
  return useAuth.getState().client;
}

/** How often each device checks whether the active-playback target changed. */
const PLAYQUEUE_POLL_MS = 15000;

/**
 * Load a saved server-side queue into local state and the engine, at its
 * saved position — shared by the launch restore and by "this device just
 * became the active player". Always lands paused unless `autoplay`, matching
 * playTrackById's pattern for a single-track load.
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
  set({ songs: remote.songs, index: idx, position: remote.positionMs / 1000 });
  const tracks = await Promise.all(remote.songs.map((s) => songToTrack(c, s, get().qualityId)));
  await engine.setQueue(tracks, idx);
  await engine.seekTo(remote.positionMs / 1000);
  if (!autoplay) await engine.pause();
  sendNowPlaying(get);
  scheduleSaveQueue(get);
}

/**
 * Restore the last-known cross-device state at launch (paused — never
 * autoplays), so opening the app on any device shows what's currently
 * playing instead of a blank player.
 */
async function restoreQueue(get: () => AudioState, set: (partial: Partial<AudioState>) => void): Promise<void> {
  const c = client();
  if (!c || !get().engine) return;
  const remote = await c.getPlayQueue().catch(() => null);
  if (!remote) return;
  set({ castTargetId: remote.targetDeviceId });
  await applyRemoteQueue(get, set, remote, false);
}

/**
 * Periodically checks whether the active-playback device changed since the
 * last check. ponytail: polling, not a push channel (SSE) — simpler, and
 * "every ~15s" is plenty fresh for a manual device switch.
 */
async function pollPlayQueue(get: () => AudioState, set: (partial: Partial<AudioState>) => void): Promise<void> {
  const c = client();
  const engine = get().engine;
  if (!c || !engine) return;
  const remote = await c.getPlayQueue().catch(() => null);
  if (!remote) return;
  const prevTarget = get().castTargetId;
  if (remote.targetDeviceId === prevTarget) return;
  set({ castTargetId: remote.targetDeviceId });
  const myId = c.getSession()?.deviceId;
  if (!remote.targetDeviceId) return; // cleared — independent mode resumes, no forced action
  if (remote.targetDeviceId === myId) {
    await applyRemoteQueue(get, set, remote, true); // I've just been designated — take over
  } else if (get().status === 'playing') {
    await engine.pause(); // handed off elsewhere — stop here to avoid double audio
  }
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
    if (get().engine) return;
    const engine = createEngine();
    await engine.setup();

    engine.on('state', (s) => {
      set({ status: s.status, index: s.index, duration: s.duration || get().duration });
    });
    engine.on('progress', (position, duration) => {
      set({ position, duration: duration || get().duration });
      maybeScrobble(get, position, duration);
      scheduleSaveQueue(get);
    });
    engine.on('trackChange', (index) => {
      scrobble = { nowPlayingSent: false, submitted: false };
      set({ index, position: 0 });
      sendNowPlaying(get);
      scheduleSaveQueue(get);
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

    // Cross-device state: show what's already playing (paused) on launch,
    // then keep checking whether another device has taken over — or handed
    // playback to this one.
    void restoreQueue(get, set);
    setInterval(() => void pollPlayQueue(get, set), PLAYQUEUE_POLL_MS);
  },

  playSongs: async (songs, startIndex = 0) => {
    const c = client();
    const engine = get().engine;
    if (!c || !engine || songs.length === 0) return;
    const tracks = await Promise.all(songs.map((s) => songToTrack(c, s, get().qualityId)));
    orderBackup = null; // new playback context invalidates any shuffle backup
    set({ songs, index: startIndex, position: 0 });
    scrobble = { nowPlayingSent: false, submitted: false };
    await engine.setQueue(tracks, startIndex);
    sendNowPlaying(get);
    scheduleSaveQueue(get);
  },

  playRadio: async (station) => {
    const engine = get().engine;
    if (!engine || !station.streamUrl) return;
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
    set({ songs: [...get().songs, ...songs] });
    await engine.add(await Promise.all(songs.map((s) => songToTrack(c, s, get().qualityId))));
  },

  toggle: async () => {
    const engine = get().engine;
    if (!engine) return;
    if (get().status === 'playing') await engine.pause();
    else await engine.play();
  },

  next: async () => {
    await get().engine?.next();
  },

  previous: async () => {
    await get().engine?.previous();
  },

  seekTo: async (seconds) => {
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
    } else if (get().status === 'playing') {
      await get().engine?.pause();
    }
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

/** Debounced savePlayQueue so the queue follows the user across devices. */
function scheduleSaveQueue(get: () => AudioState): void {
  if (saveQueueTimer) clearTimeout(saveQueueTimer);
  saveQueueTimer = setTimeout(() => {
    const c = client();
    const { songs, index, position } = get();
    if (!c || songs.length === 0 || index < 0) return;
    void c
      .savePlayQueue(
        songs.map((s) => s.id),
        songs[index]?.id,
        Math.floor(position * 1000),
      )
      .catch(() => undefined);
  }, 3000);
}
