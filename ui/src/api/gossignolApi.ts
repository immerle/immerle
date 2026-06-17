import createClient, { type Client } from 'openapi-fetch';
import type { paths, components } from './generated/schema';
import { normalizeServerUrl } from './subsonic/client';

/**
 * Typed client for the Gossignol extension API, generated from the server's
 * OpenAPI document (`src/api/generated/schema.ts`, regenerated via
 * `npm run gen:api`). Paths and payloads are fully type-checked against the
 * spec — there is no hand-maintained URL/shape duplication.
 *
 * The extension API is mounted at the server root (no `/gossignol` prefix), so
 * `baseUrl` is just the normalized server URL.
 */
export type GossignolApi = Client<paths>;

export function createGossignolApi(serverUrl: string): GossignolApi {
  return createClient<paths>({ baseUrl: normalizeServerUrl(serverUrl) });
}

// Re-export the generated DTOs that callers consume, under friendly names.
export type SetupStatus =
  paths['/setup/status']['get']['responses']['200']['content']['application/json'];
export type SetupInitRequest = components['schemas']['gossignol.SetupInitRequest'];
export type SetupInitResponse = components['schemas']['gossignol.SetupInitResponse'];
export type FieldErrorDTO = components['schemas']['gossignol.FieldErrorDTO'];
export type CapabilitiesResponse =
  paths['/capabilities']['get']['responses']['200']['content']['application/json'];

// Social / Jam DTOs.
export type FriendDTO = components['schemas']['gossignol.FriendDTO'];
export type PendingFriendDTO = components['schemas']['gossignol.PendingFriendDTO'];
export type ActivityEventDTO = components['schemas']['gossignol.ActivityEventDTO'];
export type JamSessionDTO = components['schemas']['gossignol.JamSessionDTO'];
export type JamParticipantDTO = components['schemas']['gossignol.JamParticipantDTO'];

// API tokens (personal access tokens).
export type APITokenDTO = components['schemas']['gossignol.APITokenDTO'];
export type CreateTokenResponse = components['schemas']['gossignol.CreateTokenResponse'];

// Public playlists (discovery + opt-in subscription).
export type PublicPlaylistDTO = components['schemas']['gossignol.PublicPlaylistDTO'];

// User profile (identity + recent activity + public playlists).
export type ProfileResponse = components['schemas']['gossignol.ProfileResponse'];
export type ProfilePlaylistDTO = components['schemas']['gossignol.ProfilePlaylistDTO'];

// Playlist imports from external platforms.
export type ImportDTO = components['schemas']['gossignol.ImportDTO'];
export type ImportItemDTO = components['schemas']['gossignol.ImportItemDTO'];
export type ImportSourceDTO = components['schemas']['gossignol.ImportSourceDTO'];

// Per-account UI theme.
export type ThemeDTO = components['schemas']['gossignol.ThemeDTO'];

// Admin-managed dynamic providers.
export type ProviderDTO = components['schemas']['gossignol.ProviderDTO'];

// Runtime settings + downloads cleanup (admin).
export type RuntimeSettingsDTO = components['schemas']['gossignol.RuntimeSettingsDTO'];

/** An authenticated extension-API client: layers salted-token auth on every request. */
export function createAuthedGossignolApi(
  serverUrl: string,
  auth: { t: string; s: string; v: string },
): GossignolApi {
  const api = createGossignolApi(serverUrl);
  api.use({
    onRequest({ request }) {
      const url = new URL(request.url);
      url.searchParams.set('t', auth.t);
      url.searchParams.set('s', auth.s);
      url.searchParams.set('v', auth.v);
      url.searchParams.set('f', 'json');
      return new Request(url, request);
    },
  });
  return api;
}
