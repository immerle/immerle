import { create } from 'zustand';
import {
  deriveCredentials,
  normalizeServerUrl,
  SubsonicClient,
  SubsonicCredentials,
} from '../api/subsonic/client';
import { ImmerleClient } from '../api/immerle/client';
import { probeCapabilities } from '../api/immerle/capabilities';
import { ImmerleSession } from '../api/immerle/types';
import { randomHex } from '../utils/random';
import {
  deleteSecureItem,
  getSecureItem,
  setSecureItem,
  STORAGE_KEYS,
} from './secureStore';

export type AuthStatus = 'idle' | 'restoring' | 'authenticated' | 'unauthenticated';

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
  /** Log in to an instance and persist credentials securely. */
  login: (input: {
    serverUrl: string;
    username: string;
    password: string;
  }) => Promise<void>;
  logout: () => Promise<void>;
  clearError: () => void;
}

/**
 * Build a fully-wired Immerle client from stored credentials: rebuild the
 * Subsonic client, probe capabilities, and attempt a native Immerle session
 * if the instance advertises `immerleAuth`.
 */
async function buildClient(creds: SubsonicCredentials): Promise<ImmerleClient> {
  const subsonic = new SubsonicClient(creds);
  const capabilities = await probeCapabilities(creds.serverUrl);

  // Fetch the Subsonic user record once: it carries the display name (for the
  // greeting/account UI) and the admin role.
  let displayName: string | undefined;
  let isAdmin = false;
  try {
    const me = await subsonic.getUser(creds.username);
    displayName = me.displayName;
    isAdmin = Boolean(me.adminRole);
  } catch {
    /* fall back to defaults below */
  }

  // The Immerle REST API is Bearer-only, so a native device session is required
  // for every extension call. Mint one by exchanging the Subsonic salted token
  // (no password needed — works at login and on restore). Best-effort: a plain
  // Subsonic server has no such endpoint, and capability gates hide the features.
  let session: ImmerleSession | null = null;
  if (capabilities.features.immerleAuth) {
    session = await tryImmerleLogin(subsonic, isAdmin);
  }

  const client = new ImmerleClient(subsonic, capabilities, session);
  client.setDisplayName(displayName);
  if (!session) client.setAdmin(isAdmin);
  return client;
}

/**
 * Best-effort native session: exchange the Subsonic salted token for a device
 * JWT via POST /api/v1/auth/sessions. Never throws — Subsonic auth is the floor.
 */
async function tryImmerleLogin(
  subsonic: SubsonicClient,
  isAdmin: boolean,
): Promise<ImmerleSession | null> {
  try {
    const { u, t, s, c } = subsonic.tokenParams();
    const res = await fetch(`${subsonic.serverUrl}/api/v1/auth/sessions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ username: u, token: t, salt: s, device: c }),
    });
    if (!res.ok) return null;
    const body = (await res.json()) as { token?: string; device?: { expiresAt?: string } };
    if (!body.token) return null;
    return {
      token: body.token,
      userId: '',
      username: u,
      isAdmin,
      expiresAt: body.device?.expiresAt ? Date.parse(body.device.expiresAt) : undefined,
    };
  } catch {
    return null;
  }
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
      if (!raw) {
        set({ status: 'unauthenticated', client: null });
        return;
      }
      const creds = JSON.parse(raw) as SubsonicCredentials;
      // buildClient re-mints a fresh device JWT from the stored salted token.
      const client = await buildClient(creds);
      set({ status: 'authenticated', client, displayName: client.displayName });
    } catch {
      // Corrupt/expired stored state: drop to login rather than crash.
      await deleteSecureItem(STORAGE_KEYS.credentials);
      await deleteSecureItem(STORAGE_KEYS.session);
      set({ status: 'unauthenticated', client: null });
    }
  },

  login: async ({ serverUrl, username, password }) => {
    set({ error: null });
    const normalized = normalizeServerUrl(serverUrl);
    const salt = randomHex(16);
    const creds = deriveCredentials(normalized, username, password, salt);

    // Verify the credentials actually work before persisting anything.
    const probe = new SubsonicClient(creds);
    try {
      await probe.ping();
    } catch (e) {
      set({
        status: 'unauthenticated',
        error:
          e instanceof Error
            ? `Connexion impossible : ${e.message}`
            : 'Connexion impossible',
      });
      throw e;
    }

    const client = await buildClient(creds);
    await setSecureItem(STORAGE_KEYS.credentials, JSON.stringify(creds));

    set({ status: 'authenticated', client, displayName: client.displayName, error: null });
  },

  logout: async () => {
    await deleteSecureItem(STORAGE_KEYS.credentials);
    await deleteSecureItem(STORAGE_KEYS.session);
    set({ status: 'unauthenticated', client: null, displayName: null, error: null });
  },
}));

/** Convenience selector: the live client, or throw if used before auth. */
export function useClient(): ImmerleClient {
  const client = useAuth((s) => s.client);
  if (!client) throw new Error('useClient called before authentication');
  return client;
}
