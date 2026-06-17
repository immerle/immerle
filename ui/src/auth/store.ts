import { create } from 'zustand';
import {
  deriveCredentials,
  normalizeServerUrl,
  SubsonicClient,
  SubsonicCredentials,
} from '../api/subsonic/client';
import { GossignolClient } from '../api/gossignol/client';
import { probeCapabilities } from '../api/gossignol/capabilities';
import { GossignolSession } from '../api/gossignol/types';
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
  client: GossignolClient | null;
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
 * Build a fully-wired Gossignol client from stored credentials: rebuild the
 * Subsonic client, probe capabilities, and attempt a native Gossignol session
 * if the instance advertises `gossignolAuth`.
 */
async function buildClient(
  creds: SubsonicCredentials,
  password?: string,
): Promise<GossignolClient> {
  const subsonic = new SubsonicClient(creds);
  const capabilities = await probeCapabilities(creds.serverUrl);

  let session: GossignolSession | null = null;
  if (capabilities.features.gossignolAuth && password) {
    session = await tryGossignolLogin(creds.serverUrl, creds.username, password);
  }
  const client = new GossignolClient(subsonic, capabilities, session);

  // Fetch the Subsonic user record once: it carries the display name (for the
  // greeting/account UI) and, when there's no native session, the admin role.
  try {
    const me = await subsonic.getUser(creds.username);
    client.setDisplayName(me.displayName);
    if (!session) client.setAdmin(Boolean(me.adminRole));
  } catch {
    if (!session) client.setAdmin(false);
  }
  return client;
}

/** Best-effort native session. Never throws — Subsonic auth is the floor. */
async function tryGossignolLogin(
  serverUrl: string,
  username: string,
  password: string,
): Promise<GossignolSession | null> {
  try {
    const res = await fetch(`${serverUrl}/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    if (!res.ok) return null;
    return (await res.json()) as GossignolSession;
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
      const client = await buildClient(creds);
      // Re-hydrate a persisted native session if we have one.
      const rawSession = await getSecureItem(STORAGE_KEYS.session);
      if (rawSession) client.setSession(JSON.parse(rawSession) as GossignolSession);
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

    const client = await buildClient(creds, password);
    await setSecureItem(STORAGE_KEYS.credentials, JSON.stringify(creds));
    const session = client.getSession();
    if (session) await setSecureItem(STORAGE_KEYS.session, JSON.stringify(session));

    set({ status: 'authenticated', client, displayName: client.displayName, error: null });
  },

  logout: async () => {
    await deleteSecureItem(STORAGE_KEYS.credentials);
    await deleteSecureItem(STORAGE_KEYS.session);
    set({ status: 'unauthenticated', client: null, displayName: null, error: null });
  },
}));

/** Convenience selector: the live client, or throw if used before auth. */
export function useClient(): GossignolClient {
  const client = useAuth((s) => s.client);
  if (!client) throw new Error('useClient called before authentication');
  return client;
}
