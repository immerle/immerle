import { create } from 'zustand';
import { Platform } from 'react-native';
import AsyncStorage from '@react-native-async-storage/async-storage';
import { colorScheme } from 'nativewind';
import { contrastForeground, hexToChannels, normalizeHex } from './accent';
import { useAuth } from '../auth/store';

export type ThemePreference = 'light' | 'dark' | 'system';

const STORAGE_KEY = 'immerle.theme.v1';
const ACCENT_KEY = 'immerle.accent.v1';

interface ThemeState {
  preference: ThemePreference;
  /** Custom accent hex, or null for the default green. */
  accent: string | null;
  /** True once persisted preferences have been read and applied (gates the UI). */
  hydrated: boolean;
  /** Load persisted preferences and apply them. Call once at startup. */
  hydrate: () => Promise<void>;
  setPreference: (preference: ThemePreference) => void;
  /** Set (or reset, with null) the accent color. Persists locally and to the server. */
  setAccent: (hex: string | null) => void;
  /**
   * Pull the accent from the server (source of truth across devices) and apply
   * it. Call once the session is authenticated. Local storage stays a cache.
   */
  syncFromServer: () => Promise<void>;
}

/**
 * On web, override the accent CSS variables on the document root so the change
 * reaches everything (including modals/portals). The neutral and semantic
 * variables are left untouched. (Native uses a `vars()` wrapper + `useColors`.)
 */
function applyAccentWeb(hex: string | null) {
  if (Platform.OS !== 'web' || typeof document === 'undefined') return;
  const root = document.documentElement;
  if (hex) {
    const ch = hexToChannels(hex);
    root.style.setProperty('--color-primary', ch);
    root.style.setProperty('--color-accent', ch);
    root.style.setProperty('--color-primary-foreground', hexToChannels(contrastForeground(hex)));
  } else {
    root.style.removeProperty('--color-primary');
    root.style.removeProperty('--color-accent');
    root.style.removeProperty('--color-primary-foreground');
  }
}

/**
 * Drives the app's light/dark appearance through NativeWind's `colorScheme` and
 * the user's accent color. Both persist across restarts.
 */
export const useTheme = create<ThemeState>((set) => ({
  // Dark by default — the Spotify-style look; users can switch in Réglages.
  preference: 'dark',
  accent: null,
  hydrated: false,

  hydrate: async () => {
    try {
      const stored = (await AsyncStorage.getItem(STORAGE_KEY)) as ThemePreference | null;
      const pref = stored ?? 'dark';
      colorScheme.set(pref);
      const accent = normalizeHex((await AsyncStorage.getItem(ACCENT_KEY)) ?? '');
      applyAccentWeb(accent);
      set({ preference: pref, accent, hydrated: true });
    } catch {
      colorScheme.set('dark');
      set({ hydrated: true });
    }
  },

  setPreference: (preference) => {
    colorScheme.set(preference);
    set({ preference });
    void AsyncStorage.setItem(STORAGE_KEY, preference);
  },

  setAccent: (hex) => {
    const accent = hex ? normalizeHex(hex) : null;
    applyAccentWeb(accent);
    set({ accent });
    if (accent) void AsyncStorage.setItem(ACCENT_KEY, accent);
    else void AsyncStorage.removeItem(ACCENT_KEY);
    // Persist to the server so the choice follows the user across devices.
    // Empty string clears the stored accent (server falls back to default).
    const client = useAuth.getState().client;
    if (client) void client.setTheme(accent ?? '').catch(() => {});
  },

  syncFromServer: async () => {
    const client = useAuth.getState().client;
    if (!client) return;
    try {
      const theme = await client.getTheme();
      const accent = normalizeHex(theme.accentColor ?? '');
      applyAccentWeb(accent);
      set({ accent });
      if (accent) void AsyncStorage.setItem(ACCENT_KEY, accent);
      else void AsyncStorage.removeItem(ACCENT_KEY);
    } catch {
      // Offline or unsupported: keep the locally-hydrated accent.
    }
  },
}));
