import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';

const COLLAPSED_KEY = 'immerle.sidebarCollapsed.v1';

/** Persistent UI preferences (desktop sidebar collapsed state). */
interface UIState {
  sidebarCollapsed: boolean;
  hydrate: () => Promise<void>;
  toggleSidebar: () => void;
}

export const useUI = create<UIState>((set, get) => ({
  sidebarCollapsed: false,
  hydrate: async () => {
    try {
      const v = await AsyncStorage.getItem(COLLAPSED_KEY);
      if (v != null) set({ sidebarCollapsed: v === '1' });
    } catch {
      /* keep default */
    }
  },
  toggleSidebar: () => {
    const next = !get().sidebarCollapsed;
    set({ sidebarCollapsed: next });
    void AsyncStorage.setItem(COLLAPSED_KEY, next ? '1' : '0');
  },
}));
