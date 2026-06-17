import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import { useAuth } from '../auth/store';
import { queryClient } from '../query/queryClient';
import { GossignolClient } from '../api/gossignol/client';
import { Song } from '../api/subsonic/types';
import { AudioEngine, PlayableTrack, RepeatMode } from './types';
import { createEngine } from './engine';
import { DEFAULT_QUALITY_ID, presetById } from './quality';

const QUALITY_KEY = 'gossignol.quality.v1';
const VOLUME_KEY = 'gossignol.volume.v1';

/** Fisher–Yates shuffle (pure). */
function shuffled<T>(arr: T[]): T[] {
  const a = [...arr];
  for (let i = a.length - 1; i > 0; i -= 1) {
    const j = Math.floor(Math.random() * (i + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}

/** Build a player track from a Subsonic song at the chosen quality. */
function songToTrack(client: GossignolClient, song: Song, qualityId: string): PlayableTrack {
  const preset = presetById(qualityId);
  return {
    id: song.id,
    url: client.subsonic.streamUrl(song.id, {
      maxBitRate: preset.maxBitRate || undefined,
      format: preset.format,
    }),
    title: song.title,
    artist: song.artist,
    album: song.album,
    artwork: client.subsonic.coverArtUrl(song.coverArt ?? song.id, 512),
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

  init: () => Promise<void>;
  hydrateSettings: () => Promise<void>;

  playSongs: (songs: Song[], startIndex?: number) => Promise<void>;
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

  current: () => Song | undefined;
}

// Module-scoped scrobble state (not reactive UI state).
let scrobble: ScrobbleFlags = { nowPlayingSent: false, submitted: false };
let saveQueueTimer: ReturnType<typeof setTimeout> | null = null;
// Original queue order, kept so shuffle can be turned off again.
let orderBackup: Song[] | null = null;

function client(): GossignolClient | null {
  return useAuth.getState().client;
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
    });

    await engine.setVolume(get().volume);
    set({ engine });
  },

  playSongs: async (songs, startIndex = 0) => {
    const c = client();
    const engine = get().engine;
    if (!c || !engine || songs.length === 0) return;
    const tracks = songs.map((s) => songToTrack(c, s, get().qualityId));
    orderBackup = null; // new playback context invalidates any shuffle backup
    set({ songs, index: startIndex, position: 0 });
    scrobble = { nowPlayingSent: false, submitted: false };
    await engine.setQueue(tracks, startIndex);
    sendNowPlaying(get);
    scheduleSaveQueue(get);
  },

  playTrackById: async (id, positionSec, autoplay) => {
    const c = client();
    const engine = get().engine;
    if (!c || !engine) return;
    const song = await c.subsonic.getSong(id).catch(() => ({ id, title: 'Piste' }) as Song);
    orderBackup = null;
    scrobble = { nowPlayingSent: false, submitted: false };
    set({ songs: [song], index: 0, position: positionSec });
    await engine.setQueue([songToTrack(c, song, get().qualityId)], 0);
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
    await engine.add(songs.map((s) => songToTrack(c, s, get().qualityId)));
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
    await engine.add(songs.map((s) => songToTrack(c, s, get().qualityId)));
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
    await get().engine?.seekTo(seconds);
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
    const tracks = nextSongs.map((s) => songToTrack(c, s, get().qualityId));
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
    const tracks = get().songs.map((s) => songToTrack(c, s, id));
    await engine.setQueue(tracks, idx);
    await engine.seekTo(pos);
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
  void c.subsonic.scrobble(song.id, false).catch(() => undefined);
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
  void c.subsonic
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
    void c.subsonic
      .savePlayQueue(
        songs.map((s) => s.id),
        songs[index]?.id,
        Math.floor(position * 1000),
      )
      .catch(() => undefined);
  }, 3000);
}
