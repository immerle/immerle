import createClient, { type Client } from 'openapi-fetch';
import type { paths, components } from './generated/schema';
import { normalizeServerUrl } from './subsonic/client';

/**
 * Typed client for the Immerle extension API, generated from the server's
 * OpenAPI document (`src/api/generated/schema.ts`, regenerated via
 * `npm run gen:api`). Paths and payloads are fully type-checked against the
 * spec — there is no hand-maintained URL/shape duplication.
 *
 * The extension API is mounted at the server root (no `/immerle` prefix), so
 * `baseUrl` is just the normalized server URL.
 */
export type ImmerleApi = Client<paths>;

export function createImmerleApi(serverUrl: string): ImmerleApi {
  return createClient<paths>({ baseUrl: normalizeServerUrl(serverUrl) });
}

// Re-export the generated DTOs that callers consume, under friendly names.
export type SetupStatus =
  paths['/setup/status']['get']['responses']['200']['content']['application/json'];
export type SetupInitRequest = components['schemas']['immerle.SetupInitRequest'];
export type SetupInitResponse = components['schemas']['immerle.SetupInitResponse'];
export type FieldErrorDTO = components['schemas']['immerle.FieldErrorDTO'];
export type CapabilitiesResponse =
  paths['/capabilities']['get']['responses']['200']['content']['application/json'];

// Social / Jam DTOs.
export type FriendDTO = components['schemas']['immerle.FriendDTO'];
export type PendingFriendDTO = components['schemas']['immerle.PendingFriendDTO'];
export type ActivityEventDTO = components['schemas']['immerle.ActivityEventDTO'];
export type JamSessionDTO = components['schemas']['immerle.JamSessionDTO'];
export type JamParticipantDTO = components['schemas']['immerle.JamParticipantDTO'];

// API tokens (personal access tokens).
export type APITokenDTO = components['schemas']['immerle.APITokenDTO'];
export type CreateTokenResponse = components['schemas']['immerle.CreateTokenResponse'];

// Public playlists (discovery + opt-in subscription).
export type PublicPlaylistDTO = components['schemas']['immerle.PublicPlaylistDTO'];

// User profile (identity + recent activity + public playlists).
export type ProfileResponse = components['schemas']['immerle.ProfileResponse'];
export type ProfilePlaylistDTO = components['schemas']['immerle.ProfilePlaylistDTO'];

// Playlist imports from external platforms.
export type ImportDTO = components['schemas']['immerle.ImportDTO'];
export type ImportItemDTO = components['schemas']['immerle.ImportItemDTO'];
export type ImportSourceDTO = components['schemas']['immerle.ImportSourceDTO'];

// Per-account UI theme.
export type ThemeDTO = components['schemas']['immerle.ThemeDTO'];

// Admin-managed dynamic providers.
export type ProviderDTO = components['schemas']['immerle.ProviderDTO'];

// Runtime settings + downloads cleanup (admin).
export type RuntimeSettingsDTO = components['schemas']['immerle.RuntimeSettingsDTO'];

/** An authenticated extension-API client: layers salted-token auth on every request. */
export function createAuthedImmerleApi(
  serverUrl: string,
  auth: { t: string; s: string; v: string },
): ImmerleApi {
  const api = createImmerleApi(serverUrl);
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
