import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import { Song } from '../api/subsonic/types';

const ALBUMS_KEY = 'immerle.offlineAlbums.v1';
const PLAYLISTS_KEY = 'immerle.offlinePlaylists.v1';

/** Snapshot for a full album download — unlike a lone track, keeps enough
 * metadata to render the album screen offline (not just play the songs). */
export interface OfflineAlbum {
  id: string;
  name: string;
  artist?: string;
  artistId?: string;
  year?: number;
  coverArt?: string;
  songs: Song[];
  downloadedAt: number;
}

/** Same idea for a fully-downloaded playlist. */
export interface OfflinePlaylist {
  id: string;
  name: string;
  coverArt?: string;
  songs: Song[];
  downloadedAt: number;
}

interface OfflineCatalogState {
  albums: Record<string, OfflineAlbum>;
  playlists: Record<string, OfflinePlaylist>;
  hydrated: boolean;
  hydrate: () => Promise<void>;
  saveAlbum: (album: Omit<OfflineAlbum, 'downloadedAt'>) => void;
  savePlaylist: (playlist: Omit<OfflinePlaylist, 'downloadedAt'>) => void;
  removeAlbum: (id: string) => void;
  removePlaylist: (id: string) => void;
}

function persist(key: string, value: unknown): void {
  void AsyncStorage.setItem(key, JSON.stringify(value));
}

export const useOfflineCatalog = create<OfflineCatalogState>((set) => ({
  albums: {},
  playlists: {},
  hydrated: false,

  hydrate: async () => {
    try {
      const rawAlbums = await AsyncStorage.getItem(ALBUMS_KEY);
      if (rawAlbums) set({ albums: JSON.parse(rawAlbums) as Record<string, OfflineAlbum> });
      const rawPlaylists = await AsyncStorage.getItem(PLAYLISTS_KEY);
      if (rawPlaylists) set({ playlists: JSON.parse(rawPlaylists) as Record<string, OfflinePlaylist> });
    } catch {
      /* keep default */
    }
    set({ hydrated: true });
  },

  saveAlbum: (album) => {
    set((s) => {
      const albums = { ...s.albums, [album.id]: { ...album, downloadedAt: Date.now() } };
      persist(ALBUMS_KEY, albums);
      return { albums };
    });
  },

  savePlaylist: (playlist) => {
    set((s) => {
      const playlists = { ...s.playlists, [playlist.id]: { ...playlist, downloadedAt: Date.now() } };
      persist(PLAYLISTS_KEY, playlists);
      return { playlists };
    });
  },

  removeAlbum: (id) => {
    set((s) => {
      const albums = { ...s.albums };
      delete albums[id];
      persist(ALBUMS_KEY, albums);
      return { albums };
    });
  },

  removePlaylist: (id) => {
    set((s) => {
      const playlists = { ...s.playlists };
      delete playlists[id];
      persist(PLAYLISTS_KEY, playlists);
      return { playlists };
    });
  },
}));
