import { Platform } from 'react-native';
import * as SecureStore from 'expo-secure-store';

/**
 * Cross-platform secure key/value storage.
 * Native: iOS Keychain / Android Keystore via expo-secure-store.
 * Web: falls back to `localStorage` — a softer guarantee, not hardware-backed.
 */

const isWeb = Platform.OS === 'web';

function webStorage(): Storage | null {
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
}

export async function setSecureItem(key: string, value: string): Promise<void> {
  if (isWeb) {
    webStorage()?.setItem(key, value);
    return;
  }
  await SecureStore.setItemAsync(key, value);
}

export async function getSecureItem(key: string): Promise<string | null> {
  if (isWeb) {
    return webStorage()?.getItem(key) ?? null;
  }
  return SecureStore.getItemAsync(key);
}

export async function deleteSecureItem(key: string): Promise<void> {
  if (isWeb) {
    webStorage()?.removeItem(key);
    return;
  }
  await SecureStore.deleteItemAsync(key);
}

export const STORAGE_KEYS = {
  credentials: 'immerle.credentials.v1',
  session: 'immerle.session.v1',
} as const;
