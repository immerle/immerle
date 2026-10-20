import { create } from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';

const RECENTS_KEY = 'immerle.searchRecents.v1';
const MAX_RECENTS = 8;

/** Result-type filter for the search results list; 'all' shows every type. */
export type SearchTypeFilter = 'all' | 'artist' | 'album' | 'song' | 'playlist' | 'radio';

/**
 * Shared search UI state. Drives the inline header search (web popover) and the
 * full-screen overlay (mobile) from one source: query text, open state, the
 * keyboard-highlighted result index, the result-type filter, and persisted
 * recent searches.
 */
interface SearchUIState {
  query: string;
  open: boolean;
  /** Index of the keyboard-highlighted result (desktop ↑/↓ navigation). */
  activeIndex: number;
  typeFilter: SearchTypeFilter;
  recents: string[];
  setQuery: (q: string) => void;
  openSearch: () => void;
  close: () => void;
  setActiveIndex: (i: number) => void;
  setTypeFilter: (f: SearchTypeFilter) => void;
  loadRecents: () => Promise<void>;
  addRecent: (q: string) => void;
  removeRecent: (q: string) => void;
  clearRecents: () => void;
}

export const useSearchUI = create<SearchUIState>((set, get) => ({
  query: '',
  open: false,
  activeIndex: 0,
  typeFilter: 'all',
  recents: [],
  setQuery: (query) => set({ query, open: true, activeIndex: 0 }),
  openSearch: () => set({ open: true }),
  close: () => set({ open: false, query: '', activeIndex: 0, typeFilter: 'all' }),
  setActiveIndex: (activeIndex) => set({ activeIndex }),
  setTypeFilter: (typeFilter) => set({ typeFilter, activeIndex: 0 }),

  loadRecents: async () => {
    try {
      const raw = await AsyncStorage.getItem(RECENTS_KEY);
      if (raw) {
        const parsed: unknown = JSON.parse(raw);
        if (Array.isArray(parsed)) {
          set({
            recents: parsed
              .filter((value): value is string => typeof value === 'string' && value.trim().length > 0)
              .slice(0, MAX_RECENTS),
          });
        }
      }
    } catch {
      /* ignore */
    }
  },
  addRecent: (q) => {
    const query = q.trim();
    if (!query) return;
    const recents = [query, ...get().recents.filter((r) => r !== query)].slice(0, MAX_RECENTS);
    set({ recents });
    void AsyncStorage.setItem(RECENTS_KEY, JSON.stringify(recents)).catch(() => undefined);
  },
  removeRecent: (query) => {
    const recents = get().recents.filter((recent) => recent !== query);
    if (recents.length === get().recents.length) return;
    set({ recents });
    void AsyncStorage.setItem(RECENTS_KEY, JSON.stringify(recents)).catch(() => undefined);
  },
  clearRecents: () => {
    set({ recents: [] });
    void AsyncStorage.removeItem(RECENTS_KEY).catch(() => undefined);
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
