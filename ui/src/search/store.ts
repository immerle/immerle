import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';

const RECENTS_KEY = 'gossignol.searchRecents.v1';
const MAX_RECENTS = 8;

/**
 * Shared search UI state. Drives the inline header search (web popover) and the
 * full-screen overlay (mobile) from one source: query text, open state, the
 * keyboard-highlighted result index, and persisted recent searches.
 */
interface SearchUIState {
  query: string;
  open: boolean;
  /** Index of the keyboard-highlighted result (desktop ↑/↓ navigation). */
  activeIndex: number;
  recents: string[];
  setQuery: (q: string) => void;
  openSearch: () => void;
  close: () => void;
  setActiveIndex: (i: number) => void;
  loadRecents: () => Promise<void>;
  addRecent: (q: string) => void;
}

export const useSearchUI = create<SearchUIState>((set, get) => ({
  query: '',
  open: false,
  activeIndex: 0,
  recents: [],
  setQuery: (query) => set({ query, open: true, activeIndex: 0 }),
  openSearch: () => set({ open: true }),
  close: () => set({ open: false, query: '', activeIndex: 0 }),
  setActiveIndex: (activeIndex) => set({ activeIndex }),

  loadRecents: async () => {
    try {
      const raw = await AsyncStorage.getItem(RECENTS_KEY);
      if (raw) set({ recents: JSON.parse(raw) as string[] });
    } catch {
      /* ignore */
    }
  },
  addRecent: (q) => {
    const query = q.trim();
    if (!query) return;
    const recents = [query, ...get().recents.filter((r) => r !== query)].slice(0, MAX_RECENTS);
    set({ recents });
    void AsyncStorage.setItem(RECENTS_KEY, JSON.stringify(recents));
  },
}));

/**
 * Module-level bridge for keyboard navigation: `SearchResults` publishes the
 * current flat result count and a selector here each render, and `SearchOverlay`
 * (which owns the keydown listener) reads it on ↑/↓/Enter. Kept off the store to
 * avoid re-renders on every results change.
 */
export const searchNav: { count: number; selectAt: (i: number) => void } = {
  count: 0,
  selectAt: () => {},
};
