import { createImmerleApi, CapabilitiesResponse } from '../immerleApi';
import { Capabilities } from './types';

/**
 * Conservative defaults assumed when an instance does NOT expose the Immerle
 * capabilities endpoint — i.e. a plain Subsonic/OpenSubsonic server. Only the
 * standard Subsonic surface is enabled, so the UI degrades gracefully to a
 * pure music client.
 */
export const SUBSONIC_ONLY_CAPABILITIES: Capabilities = {
  version: 'subsonic-compatible',
  apiRevision: 0,
  features: {
    immerleAuth: false,
    onDemandCatalog: false,
    dynamicProviders: false,
    runtimeSettings: false,
    cleanup: false,
    federation: false,
    jam: false,
    collaborativePlaylists: false,
    publicPlaylists: false,
    playlistImport: false,
    social: false,
    libraryAdmin: false,
    adminExtended: false,
    offlineDownloads: false,
    internetRadio: false,
    wrapped: false,
  },
};

/**
 * Probe an instance for Immerle capabilities via the generated client
 * (`GET <server>/capabilities`). Returns the conservative Subsonic-only set on
 * any failure (network error, 404, malformed body) so the app always has a
 * usable capability map. This is what makes the client "capability-aware":
 * features are masked unless explicitly advertised.
 */
export async function probeCapabilities(
  serverUrl: string,
  signal?: AbortSignal,
): Promise<Capabilities> {
  try {
    const api = createImmerleApi(serverUrl);
    const { data, error } = await api.GET('/capabilities', { signal });
    if (error || !data) return SUBSONIC_ONLY_CAPABILITIES;
    return adaptCapabilities(data);
  } catch {
    return SUBSONIC_ONLY_CAPABILITIES;
  }
}

/**
 * Adapt the server's capability map (`{ [feature]: { version, ... } }`) to the
 * app's boolean feature flags. A feature is enabled when the server advertises
 * its key; everything unadvertised stays off.
 */
export function adaptCapabilities(payload: CapabilitiesResponse): Capabilities {
  const caps = (payload.capabilities ?? {}) as Record<string, unknown>;
  const has = (key: string) => Object.prototype.hasOwnProperty.call(caps, key);
  return {
    version: payload.protocolVersion ?? SUBSONIC_ONLY_CAPABILITIES.version,
    apiRevision: 1,
    features: {
      // The server advertises native device auth (JWT) via the `devices` cap.
      immerleAuth: has('devices'),
      onDemandCatalog: has('onDemandCatalog'),
      dynamicProviders: has('dynamicProviders'),
      runtimeSettings: has('runtimeSettings'),
      cleanup: has('cleanup'),
      federation: has('federation'),
      jam: has('jam'),
      collaborativePlaylists: has('collaborativePlaylists'),
      publicPlaylists: has('publicPlaylists'),
      playlistImport: has('playlistImport'),
      social: has('friendships') || has('activityFeed') || has('shares'),
      libraryAdmin: has('libraryAdmin'),
      adminExtended: has('admin') || has('adminExtended'),
      offlineDownloads: has('offlineDownloads'),
      internetRadio: has('internetRadio'),
      wrapped: has('wrapped'),
    },
  };
}
