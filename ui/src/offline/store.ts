import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import * as Network from 'expo-network';

import { Song } from '../api/subsonic/types';
import { useAuth } from '../auth/store';
import * as fs from './fs';
import { offlineFileName } from './paths';

const KEY = 'immerle.offline.v1';
const WIFI_KEY = 'immerle.offlineWifiOnly.v1';

/** True when downloads should be blocked on a cellular connection. */
async function onCellular(): Promise<boolean> {
  try {
    const state = await Network.getNetworkStateAsync();
    return state.type === Network.NetworkStateType.CELLULAR;
  } catch {
    return false; // can't tell (e.g. web) → don't block
  }
}

function isQuotaError(e: unknown): boolean {
  return e instanceof Error && e.name === 'QuotaExceededError';
}

/** A completed offline download (the registry entry, persisted to AsyncStorage). */
export interface OfflineEntry {
  id: string;
  /** Basename under the offline dir; the absolute URI is rebuilt via fs.fileUri. */
  file: string;
  title: string;
  artist?: string;
  album?: string;
  coverArt?: string;
  duration?: number;
  /** On-disk size in bytes. */
  size?: number;
  downloadedAt: number;
}

interface DownloadsState {
  /** Completed downloads, keyed by track id. */
  entries: Record<string, OfflineEntry>;
  /** In-flight downloads, id -> 0..1. Ephemeral (not persisted). */
  progress: Record<string, number>;
  /** Only download over Wi-Fi (skip on cellular). Persisted. */
  wifiOnly: boolean;
  /** Last surfaced error, or null. 'quota' = device/browser storage is full. */
  lastError: 'quota' | null;
  hydrated: boolean;
  hydrate: () => Promise<void>;
  setWifiOnly: (v: boolean) => void;
  clearError: () => void;
  /** Download a track for offline playback (no-op on web / when disabled). */
  download: (song: Song) => Promise<void>;
  /** Download several tracks in sequence (already-downloaded ones are skipped). */
  downloadMany: (songs: Song[]) => Promise<void>;
  /** Delete a downloaded track (file + registry entry). */
  remove: (id: string) => Promise<void>;
  /** Delete every downloaded track. */
  clearAll: () => Promise<void>;
}

function persist(entries: Record<string, OfflineEntry>): void {
  void AsyncStorage.setItem(KEY, JSON.stringify(entries));
}

export const useDownloads = create<DownloadsState>((set, get) => ({
  entries: {},
  progress: {},
  wifiOnly: false,
  lastError: null,
  hydrated: false,

  hydrate: async () => {
    try {
      const raw = await AsyncStorage.getItem(KEY);
      if (raw) set({ entries: JSON.parse(raw) as Record<string, OfflineEntry> });
      const wifi = await AsyncStorage.getItem(WIFI_KEY);
      if (wifi != null) set({ wifiOnly: wifi === '1' });
    } catch {
      /* keep default */
    }
    set({ hydrated: true });
  },

  setWifiOnly: (v) => {
    set({ wifiOnly: v });
    void AsyncStorage.setItem(WIFI_KEY, v ? '1' : '0');
  },

  clearError: () => set({ lastError: null }),

  download: async (song) => {
    if (!fs.isSupported) return;
    const client = useAuth.getState().client;
    if (!client || !client.has('offlineDownloads')) return;
    const id = song.id;
    // Already downloaded or in flight — nothing to do.
    if (get().entries[id] || get().progress[id] != null) return;
    // Respect the Wi-Fi-only preference (no-op on web, where type is unknown).
    if (get().wifiOnly && (await onCellular())) return;

    set((s) => ({ progress: { ...s.progress, [id]: 0 } }));
    try {
      const url = await client.downloadUrl(id);
      const file = offlineFileName(id, song.suffix);
      const size = await fs.download(url, file, (p) => set((s) => ({ progress: { ...s.progress, [id]: p } })));
      const entry: OfflineEntry = {
        id,
        file,
        title: song.title,
        artist: song.artist,
        album: song.album,
        coverArt: song.coverArt,
        duration: song.duration,
        size,
        downloadedAt: Date.now(),
      };
      set((s) => {
        const progress = { ...s.progress };
        delete progress[id];
        const entries = { ...s.entries, [id]: entry };
        persist(entries);
        return { entries, progress };
      });
    } catch (e) {
      // Drop the progress marker; leave no partial entry behind. Surface a full
      // storage so the user understands why the download didn't stick.
      set((s) => {
        const progress = { ...s.progress };
        delete progress[id];
        return isQuotaError(e) ? { progress, lastError: 'quota' as const } : { progress };
      });
    }
  },

  downloadMany: async (songs) => {
    // Sequential so a "download album/playlist" doesn't fire dozens of parallel
    // fetches. download() already skips ones that are done or in flight.
    for (const song of songs) {
      await get().download(song);
    }
  },

  remove: async (id) => {
    const entry = get().entries[id];
    if (!entry) return;
    try {
      await fs.remove(entry.file);
    } catch {
      /* best effort: still drop the registry entry */
    }
    set((s) => {
      const entries = { ...s.entries };
      delete entries[id];
      persist(entries);
      return { entries };
    });
  },

  clearAll: async () => {
    const entries = get().entries;
    await Promise.all(Object.values(entries).map((e) => fs.remove(e.file).catch(() => {})));
    set({ entries: {} });
    persist({});
  },
}));

/**
 * Non-reactive: a playable URL for a downloaded track, or null if it isn't
 * downloaded. Async because the web backend resolves a blob: URL from IndexedDB
 * (native returns the local file:// path). The player awaits this to play offline
 * copies instead of streaming.
 */
export async function offlinePlayableUrl(id: string): Promise<string | null> {
  const e = useDownloads.getState().entries[id];
  return e ? fs.playableUrl(e.file) : null;
}
