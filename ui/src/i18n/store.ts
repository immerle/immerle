import AsyncStorage from '@react-native-async-storage/async-storage';
import { getLocales } from 'expo-localization';
import { create } from 'zustand';

import { i18n } from './index';

const KEY = 'immerle.locale.v1';

/** Language preference: `system` follows the device, otherwise a fixed locale. */
export type LocalePref = 'system' | 'en' | 'fr';

function deviceLocale(): string {
  return getLocales()[0]?.languageCode ?? 'en';
}

/** Point the i18n instance at the locale a preference resolves to. */
function apply(pref: LocalePref) {
  i18n.locale = pref === 'system' ? deviceLocale() : pref;
}

interface LocaleState {
  preference: LocalePref;
  hydrate: () => Promise<void>;
  setPreference: (p: LocalePref) => void;
}

/**
 * Local language preference (persisted on-device). Server-side sync will layer
 * on top of this later; for now it's the single source of truth.
 */
export const useLocale = create<LocaleState>((set) => ({
  preference: 'system',
  hydrate: async () => {
    try {
      const v = (await AsyncStorage.getItem(KEY)) as LocalePref | null;
      if (v) {
        set({ preference: v });
        apply(v);
      }
    } catch {
      /* keep the device default */
    }
  },
  setPreference: (p) => {
    set({ preference: p });
    apply(p);
    void AsyncStorage.setItem(KEY, p);
  },
}));
