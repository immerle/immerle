import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';

import { Song } from '../api/subsonic/types';
import { useAuth } from '../auth/store';
import * as fs from './fs';
import { offlineFileName } from './paths';

const KEY = 'immerle.offline.v1';

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
  hydrated: boolean;
  hydrate: () => Promise<void>;
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
  hydrated: false,

  hydrate: async () => {
    try {
      const raw = await AsyncStorage.getItem(KEY);
      if (raw) set({ entries: JSON.parse(raw) as Record<string, OfflineEntry> });
    } catch {
      /* keep default */
    }
    set({ hydrated: true });
  },

  download: async (song) => {
    if (!fs.isSupported) return;
    const client = useAuth.getState().client;
    if (!client || !client.has('offlineDownloads')) return;
    const id = song.id;
    // Already downloaded or in flight — nothing to do.
    if (get().entries[id] || get().progress[id] != null) return;

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
    } catch {
      // Drop the progress marker; leave no partial entry behind.
      set((s) => {
        const progress = { ...s.progress };
        delete progress[id];
        return { progress };
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
