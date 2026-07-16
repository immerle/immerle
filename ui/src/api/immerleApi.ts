import createClient, { type Client } from 'openapi-fetch';
import type { paths, components } from './generated/schema';
import { normalizeServerUrl } from '../utils/serverUrl';

/**
 * Typed client for the Immerle REST API, generated from the server's OpenAPI
 * document (`src/api/generated/schema.ts`, regenerated via `npm run gen:api`).
 * Paths and payloads are fully type-checked against the spec.
 *
 * The REST API is mounted under `/api/v1`; authenticated endpoints require a
 * Bearer token (a device JWT or a personal API token).
 */
export type ImmerleApi = Client<paths>;

/** API base path appended to the normalized server URL. */
const API_BASE = '/api/v1';

function baseUrl(serverUrl: string): string {
  return normalizeServerUrl(serverUrl) + API_BASE;
}

export function createImmerleApi(serverUrl: string): ImmerleApi {
  return createClient<paths>({ baseUrl: baseUrl(serverUrl) });
}

/** An authenticated REST client: sends `Authorization: Bearer <token>` on every request. */
export function createAuthedImmerleApi(serverUrl: string, token: string): ImmerleApi {
  const api = createImmerleApi(serverUrl);
  api.use({
    onRequest({ request }) {
      request.headers.set('Authorization', `Bearer ${token}`);
      return request;
    },
  });
  return api;
}

// Re-export the generated DTOs that callers consume, under friendly names.
export type SetupStatus =
  paths['/setup']['get']['responses']['200']['content']['application/json'];
export type SetupInitRequest = components['schemas']['immerle.SetupInitRequest'];
export type SetupInitResponse = components['schemas']['immerle.UserDTO'];
export type FieldErrorDTO = components['schemas']['immerle.fieldError'];
export type ApiError = components['schemas']['immerle.apiError'];
/** The `{error:{code,message,params}}` envelope returned on every non-2xx. */
export type ErrorResponse = components['schemas']['immerle.errorResponse'];
export type CapabilitiesResponse =
  paths['/capabilities']['get']['responses']['200']['content']['application/json'];

// Catalog browse DTOs (the native shapes that replace the Subsonic ones).
export type ArtistView = components['schemas']['immerle.artistView'];
export type AlbumView = components['schemas']['immerle.albumView'];
export type SongView = components['schemas']['immerle.songView'];
export type GenreView = components['schemas']['immerle.genreView'];
export type FavoritesView = components['schemas']['immerle.favoritesView'];
export type SearchView = components['schemas']['immerle.searchView'];
export type PlaylistView = components['schemas']['immerle.playlistView'];
export type HallOfFameView = components['schemas']['immerle.hallOfFameView'];
export type NowPlayingView = components['schemas']['immerle.nowPlayingView'];
export type AdminUserView = components['schemas']['immerle.adminUserView'];
export type StationView = components['schemas']['immerle.stationView'];
export type PlayQueueView = components['schemas']['immerle.playQueueView'];

// Session creation (device login).
export type LoginResponse = components['schemas']['immerle.LoginDTO'];

// Social / Jam DTOs.
export type FriendDTO = components['schemas']['immerle.FriendDTO'];
export type PendingFriendDTO = components['schemas']['immerle.PendingFriendDTO'];
export type ActivityEventDTO = components['schemas']['immerle.ActivityEventDTO'];
export type JamSessionDTO = components['schemas']['immerle.JamSessionDTO'];
export type JamParticipantDTO = components['schemas']['immerle.JamParticipantDTO'];

// API tokens (personal access tokens). Device-flagged ones (isDevice) back
// the "Connected devices" screen — see APITokenRepo.ListDeviceSessions.
export type APITokenDTO = components['schemas']['immerle.APITokenDTO'];
export type CreateTokenResponse = components['schemas']['immerle.CreateTokenDTO'];

// Public playlists (discovery + opt-in subscription).
export type PublicPlaylistDTO = components['schemas']['immerle.PublicPlaylistDTO'];

// User profile (identity + recent activity + public playlists).
export type ProfileResponse = components['schemas']['immerle.ProfileDTO'];
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
