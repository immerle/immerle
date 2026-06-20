import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';

const COLLAPSED_KEY = 'immerle.sidebarCollapsed.v1';

/** Persistent UI preferences (desktop sidebar collapsed state) + transient
 * mobile drawer state. */
interface UIState {
  sidebarCollapsed: boolean;
  /** Whether the mobile slide-in sidebar drawer is open (not persisted). */
  drawerOpen: boolean;
  openDrawer: () => void;
  closeDrawer: () => void;
  hydrate: () => Promise<void>;
  toggleSidebar: () => void;
}

export const useUI = create<UIState>((set, get) => ({
  sidebarCollapsed: false,
  drawerOpen: false,
  openDrawer: () => set({ drawerOpen: true }),
  closeDrawer: () => set({ drawerOpen: false }),
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
