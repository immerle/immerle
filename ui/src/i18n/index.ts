import { getLocales } from 'expo-localization';
import { I18n } from 'i18n-js';

import { ImmerleApiError } from '../api/immerle/types';
import en from './locales/en.json';
import fr from './locales/fr.json';

/**
 * App-wide i18n instance. Locale is taken from the device; missing keys fall
 * back to English. Interpolation uses `{{var}}` placeholders, filled from the
 * options object (or, for API errors, the server-sent `params`).
 */
export const i18n = new I18n({ en, fr });
i18n.enableFallback = true;
i18n.defaultLocale = 'en';
i18n.locale = getLocales()[0]?.languageCode ?? 'en';

/** Translate a key, e.g. `t('errors.not_found')`. Thin bound re-export. */
export const t = i18n.t.bind(i18n);

/**
 * Localize any thrown value for display. API errors are keyed by their server
 * `code` (`errors.<code>`) with the server's `params` interpolated; the raw
 * server message is the fallback when a code has no translation yet.
 */
export function tError(err: unknown): string {
  if (err instanceof ImmerleApiError && err.code) {
    return i18n.t(`errors.${err.code}`, { defaultValue: err.message || err.code, ...err.params });
  }
  if (err instanceof Error && err.message) {
    return i18n.t(`errors.${err.message}`, { defaultValue: err.message });
  }
  return i18n.t('errors.unknown', { defaultValue: 'Something went wrong.' });
}
