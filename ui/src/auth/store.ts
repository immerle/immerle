import { Platform } from 'react-native';
import { create } from 'zustand';
import { normalizeServerUrl } from '../utils/serverUrl';
import { createAuthedImmerleApi, createImmerleApi } from '../api/immerleApi';
import { ImmerleClient } from '../api/immerle/client';
import { probeCapabilities } from '../api/immerle/capabilities';
import { ImmerleSession } from '../api/immerle/types';
import {
  deleteSecureItem,
  getSecureItem,
  setSecureItem,
  STORAGE_KEYS,
} from './secureStore';

export type AuthStatus = 'idle' | 'restoring' | 'authenticated' | 'unauthenticated';

/** Persisted native credentials: a personal API token (no password is stored). */
interface StoredAuth {
  serverUrl: string;
  username: string;
  /** The personal API token (gsk_…) used as the Bearer credential. */
  apiToken: string;
  /** Its id, so logout can revoke it server-side. */
  tokenId: string;
}

/**
 * Friendly device label for the "cast to device" picker, e.g. "iPhone/iPad",
 * "Chrome (Mac)". Language-neutral on purpose — it's a generated name, not UI copy.
 */
function deviceLabel(): string {
  if (Platform.OS === 'ios') return 'iPhone/iPad';
  if (Platform.OS === 'android') return 'Android';
  const ua = typeof navigator !== 'undefined' ? navigator.userAgent : '';
  const browser = /Edg\//.test(ua) ? 'Edge' : /Chrome\//.test(ua) ? 'Chrome' : /Firefox\//.test(ua) ? 'Firefox' : /Safari\//.test(ua) ? 'Safari' : 'Web';
  const os = /Windows/.test(ua) ? 'Windows' : /Mac OS/.test(ua) ? 'Mac' : /Linux/.test(ua) ? 'Linux' : '';
  return os ? `${browser} (${os})` : browser;
}

interface AuthState {
  status: AuthStatus;
  client: ImmerleClient | null;
  /** Reactive display name of the current user (falls back to the username). */
  displayName: string | null;
  error: string | null;
  /** Restore a persisted session at app start. Idempotent. */
  restore: () => Promise<void>;
  /** Update the cached display name (after a self-service account edit). */
  setDisplayName: (name: string | null) => void;
  /** Log in to an instance and persist a personal API token securely. */
  login: (input: {
    serverUrl: string;
    username: string;
    password: string;
  }) => Promise<void>;
  logout: () => Promise<void>;
  clearError: () => void;
}

/**
 * Native login: trade the password for a device JWT (POST /auth/sessions), then
 * mint a long-lived personal API token (POST /tokens). Only the token is
 * persisted — the password never is, and no re-mint is needed on restore.
 */
async function nativeLogin(serverUrl: string, username: string, password: string): Promise<StoredAuth> {
  const pub = createImmerleApi(serverUrl);
  const label = deviceLabel();
  const login = await pub.POST('/auth/sessions', { body: { username, password, device: label } });
  if (login.error || !login.data?.token) {
    throw new Error(login.error?.error?.message ?? 'invalid credentials');
  }
  const authed = createAuthedImmerleApi(serverUrl, login.data.token);
  const tok = await authed.POST('/tokens', { body: { name: label, device: true } });
  if (tok.error || !tok.data?.token) {
    throw new Error(tok.error?.error?.message ?? 'could not create an access token');
  }
  return { serverUrl, username, apiToken: tok.data.token, tokenId: tok.data.id ?? '' };
}

/**
 * Build a fully-wired client from stored native credentials: probe capabilities
 * and fetch the account record (for the display name and admin role).
 */
async function buildClient(stored: StoredAuth): Promise<ImmerleClient> {
  const capabilities = await probeCapabilities(stored.serverUrl);
  const session: ImmerleSession = {
    token: stored.apiToken,
    userId: '',
    username: stored.username,
    isAdmin: false,
    deviceId: stored.tokenId || undefined,
  };
  const client = new ImmerleClient(stored.serverUrl, stored.username, capabilities, session);
  try {
    const me = await client.getAccount();
    client.setDisplayName(me.displayName);
    client.setSession({ ...session, isAdmin: Boolean(me.isAdmin) });
  } catch {
    /* keep defaults (display name falls back to the username) */
  }
  return client;
}

export const useAuth = create<AuthState>((set, get) => ({
  status: 'idle',
  client: null,
  displayName: null,
  error: null,

  clearError: () => set({ error: null }),

  setDisplayName: (name) => {
    get().client?.setDisplayName(name ?? undefined);
    set({ displayName: name });
  },

  restore: async () => {
    if (get().status === 'authenticated' || get().status === 'restoring') return;
    set({ status: 'restoring', error: null });
    try {
      const raw = await getSecureItem(STORAGE_KEYS.credentials);
      const stored = raw ? (JSON.parse(raw) as Partial<StoredAuth>) : null;
      // No token (or a legacy Subsonic credential blob): require a fresh login.
      if (!stored?.apiToken || !stored.serverUrl || !stored.username) {
        if (raw) await deleteSecureItem(STORAGE_KEYS.credentials);
        set({ status: 'unauthenticated', client: null });
        return;
      }
      const client = await buildClient(stored as StoredAuth);
      set({ status: 'authenticated', client, displayName: client.displayName });
    } catch {
      await deleteSecureItem(STORAGE_KEYS.credentials);
      await deleteSecureItem(STORAGE_KEYS.session);
      set({ status: 'unauthenticated', client: null });
    }
  },

  login: async ({ serverUrl, username, password }) => {
    set({ error: null });
    const normalized = normalizeServerUrl(serverUrl);
    let stored: StoredAuth;
    try {
      stored = await nativeLogin(normalized, username, password);
    } catch (e) {
      set({
        status: 'unauthenticated',
        error: e instanceof Error ? `Connexion impossible : ${e.message}` : 'Connexion impossible',
      });
      throw e;
    }

    const client = await buildClient(stored);
    await setSecureItem(STORAGE_KEYS.credentials, JSON.stringify(stored));
    set({ status: 'authenticated', client, displayName: client.displayName, error: null });
  },

  logout: async () => {
    // Best-effort server-side revocation of the stored token.
    try {
      const raw = await getSecureItem(STORAGE_KEYS.credentials);
      const stored = raw ? (JSON.parse(raw) as Partial<StoredAuth>) : null;
      if (stored?.tokenId) await get().client?.revokeToken(stored.tokenId).catch(() => undefined);
    } catch {
      /* ignore */
    }
    await deleteSecureItem(STORAGE_KEYS.credentials);
    await deleteSecureItem(STORAGE_KEYS.session);
    set({ status: 'unauthenticated', client: null, displayName: null, error: null });
  },
}));
