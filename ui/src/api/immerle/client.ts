import { SubsonicClient } from '../subsonic/client';
import {
  ActivityEventDTO,
  APITokenDTO,
  createAuthedImmerleApi,
  CreateTokenResponse,
  FriendDTO,
  ImmerleApi,
  JamParticipantDTO,
  ImportDTO,
  ImportItemDTO,
  ImportSourceDTO,
  JamSessionDTO,
  PendingFriendDTO,
  ProfilePlaylistDTO,
  ProfileResponse,
  ProviderDTO,
  PublicPlaylistDTO,
  RuntimeSettingsDTO,
  ThemeDTO,
} from '../immerleApi';
import {
  CapabilityFeature,
  Capabilities,
  DownloadJob,
  FederationState,
  ImmerleApiError,
  ImmerleSession,
  LibraryStats,
  Provider,
  ScanProgress,
  ServerSettings,
  TranscodeProfile,
} from './types';

/** Normalize a provider DTO (all fields optional) into a complete {@link Provider}. */
function toProvider(dto: ProviderDTO): Provider {
  return {
    name: dto.name ?? '',
    kind: dto.kind ?? 'http',
    endpoint: dto.endpoint ?? '',
    config: dto.config ?? '{}',
    enabled: dto.enabled ?? false,
    active: dto.active ?? false,
    builtin: dto.builtin ?? false,
    deletable: dto.deletable ?? true,
    sortOrder: dto.sortOrder ?? 0,
  };
}

/**
 * Capability-aware client that composes the raw Subsonic surface with the
 * extended Immerle REST API.
 *
 * Standard music operations are delegated to the embedded {@link SubsonicClient}
 * (`api.subsonic.*`). Immerle-specific operations (extended admin, providers,
 * federation, on-demand catalog) live here and are guarded by {@link has} so the
 * UI can hide what the instance does not advertise.
 *
 * Endpoints are mounted at the server root (`<<serverUrl>>/...`, no prefix) and
 * carry the Subsonic salted-token query params for auth, plus the Immerle
 * bearer token when a native session exists.
 */
export class ImmerleClient {
  /** Admin status derived from the Subsonic user's adminRole (set post-construction). */
  private adminFlag = false;
  /** Free-text display name (set post-construction); falls back to the username. */
  private displayNameValue?: string;
  private _api?: ImmerleApi;

  constructor(
    public readonly subsonic: SubsonicClient,
    public readonly capabilities: Capabilities,
    private session: ImmerleSession | null = null,
  ) {}

  /** Authenticated, typed extension-API client (lazy). */
  private get api(): ImmerleApi {
    if (!this._api) {
      const { t, s, v } = this.subsonic.tokenParams();
      this._api = createAuthedImmerleApi(this.serverUrl, { t, s, v });
    }
    return this._api;
  }

  /** Auth query params required by the typed endpoints (`u`, `c`). */
  private q(): { u: string; c: string } {
    const { u, c } = this.subsonic.tokenParams();
    return { u, c };
  }

  get serverUrl(): string {
    return this.subsonic.serverUrl;
  }

  get username(): string {
    return this.subsonic.username;
  }

  /** Name to show in the UI: the display name if set, else the username. */
  get displayName(): string {
    return this.displayNameValue?.trim() || this.username;
  }

  setDisplayName(name?: string) {
    this.displayNameValue = name;
  }

  /** True when the connected account is an admin (native session or Subsonic role). */
  get isAdmin(): boolean {
    return this.session?.isAdmin ?? this.adminFlag;
  }

  setAdmin(isAdmin: boolean) {
    this.adminFlag = isAdmin;
  }

  setSession(session: ImmerleSession | null) {
    this.session = session;
  }

  getSession(): ImmerleSession | null {
    return this.session;
  }

  /** Capability gate. Use everywhere a Immerle-only feature is offered. */
  has(feature: CapabilityFeature): boolean {
    return this.capabilities.features[feature];
  }

  // --- Low-level Immerle REST helper ------------------------------------

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
    signal?: AbortSignal,
  ): Promise<T> {
    // The extension API is mounted at the server root (no `/immerle` prefix).
    // Reuse Subsonic auth params so the server can authorize without a second
    // login; layer the Immerle bearer token on top when present.
    const url = this.subsonic.authedUrl(path);

    const headers: Record<string, string> = { Accept: 'application/json' };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (this.session?.token) headers.Authorization = `Bearer ${this.session.token}`;

    const res = await fetch(url, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal,
    });
    if (!res.ok) {
      let message = `HTTP ${res.status}`;
      try {
        const j = (await res.json()) as { message?: string; error?: string };
        message = j.message ?? j.error ?? message;
      } catch {
        /* ignore non-JSON error bodies */
      }
      throw new ImmerleApiError(res.status, message);
    }
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  // --- Admin: library ------------------------------------------------------

  /**
   * Library-wide stats (counts + on-disk size in bytes) from `/library/stats`.
   * Falls back to deriving counts from Subsonic on a plain server (no size).
   */
  async getLibraryStats(signal?: AbortSignal): Promise<LibraryStats> {
    try {
      const { data, error } = await this.api.GET('/library/stats', {
        params: { query: this.q() },
        signal,
      });
      if (!error && data?.stats) {
        const s = data.stats;
        return {
          artistCount: s.artists ?? 0,
          albumCount: s.albums ?? 0,
          songCount: s.tracks ?? 0,
          totalSize: s.totalSize ?? 0,
          lastScan: s.updatedAt,
        };
      }
    } catch {
      /* fall through to the Subsonic-derived counts */
    }
    // Degraded path: best-effort counts via Subsonic (no on-disk size).
    const [artists, albums] = await Promise.all([
      this.subsonic.getArtists(),
      this.subsonic.getAlbumList('alphabeticalByName', { size: 500 }),
    ]);
    return {
      artistCount: artists.length,
      albumCount: albums.length,
      songCount: albums.reduce((n, a) => n + (a.songCount ?? 0), 0),
      totalSize: 0,
    };
  }

  /** Trigger a scan. `full=false` requests an incremental scan when supported. */
  async startScan(full = false): Promise<ScanProgress> {
    if (this.has('adminExtended')) {
      return this.request<ScanProgress>('POST', 'admin/library/scan', { full });
    }
    const s = await this.subsonic.startScan();
    return { scanning: s.scanning, count: s.count ?? 0, phase: 'scanning' };
  }

  async getScanProgress(signal?: AbortSignal): Promise<ScanProgress> {
    if (this.has('adminExtended')) {
      return this.request<ScanProgress>('GET', 'admin/library/scan', undefined, signal);
    }
    const s = await this.subsonic.getScanStatus();
    return {
      scanning: s.scanning,
      count: s.count ?? 0,
      phase: s.scanning ? 'scanning' : 'idle',
    };
  }

  // --- Admin: dynamic providers (typed via OpenAPI) -----------------------

  /** List configured providers (with live `enabled`/`active` status). Admin-only. */
  async listProviders(signal?: AbortSignal): Promise<Provider[]> {
    const { data, error } = await this.api.GET('/admin/providers', {
      params: { query: this.q() },
      signal,
    });
    if (error || !data) throw new ImmerleApiError(0, 'providers_failed');
    return (data.providers ?? []).map(toProvider);
  }

  /**
   * Create or update a provider. Applied immediately: enabled → registered live,
   * disabled → removed. `config` is an opaque JSON string. Returns the full list.
   */
  async upsertProvider(p: {
    name: string;
    endpoint: string;
    config?: string;
    enabled?: boolean;
    kind?: string;
  }): Promise<Provider[]> {
    const { data, error } = await this.api.POST('/admin/providers', {
      params: { query: { ...this.q(), ...p } },
    });
    if (error || !data) throw new ImmerleApiError(0, 'provider_upsert_failed');
    return (data.providers ?? []).map(toProvider);
  }

  /** Toggle a provider on/off; applied to the live registry immediately. */
  async setProviderEnabled(name: string, enabled: boolean): Promise<Provider> {
    const { data, error } = await this.api.POST('/admin/providers/enable', {
      params: { query: { ...this.q(), name, enabled } },
    });
    if (error || !data?.provider) throw new ImmerleApiError(0, 'provider_enable_failed');
    return toProvider(data.provider);
  }

  /** Delete a provider config and unregister it. Built-ins are not deletable. */
  async deleteProvider(name: string): Promise<void> {
    const { error } = await this.api.POST('/admin/providers/delete', {
      params: { query: { ...this.q(), name } },
    });
    if (error) throw new ImmerleApiError(0, 'provider_delete_failed');
  }

  /**
   * Set the provider priority order (lower = higher priority). `order` must list
   * every provider name exactly once. Order also drives the search fallback.
   */
  async reorderProviders(order: string[]): Promise<Provider[]> {
    const { data, error } = await this.api.POST('/admin/providers/reorder', {
      params: { query: { ...this.q(), order: order.join(',') } },
    });
    if (error || !data) throw new ImmerleApiError(0, 'provider_reorder_failed');
    return (data.providers ?? []).map(toProvider);
  }

  // --- Admin: runtime settings --------------------------------------------

  /** Current runtime settings, plus whether a restart is pending. */
  async getSettings(signal?: AbortSignal): Promise<SettingsResult> {
    const data = await this.request<SettingsResponseRaw>('GET', 'admin/settings', undefined, signal);
    return {
      settings: data.settings ?? {},
      restartRequired: data.restartRequired ?? false,
      pendingRestart: data.pendingRestart ?? [],
    };
  }

  /**
   * Apply a partial settings update (send only the sub-objects that changed).
   * Uses a plain `fetch` (not the typed client): this is the only extension call
   * with a JSON body, and re-wrapping a body-carrying Request in the auth
   * middleware made Chrome fail the connection (ERR_ALPN_NEGOTIATION_FAILED).
   */
  async updateSettings(patch: RuntimeSettingsDTO): Promise<SettingsResult> {
    const data = await this.request<SettingsResponseRaw>('POST', 'admin/settings', patch);
    return {
      settings: data.settings ?? {},
      restartRequired: data.restartRequired ?? false,
      pendingRestart: data.pendingRestart ?? [],
    };
  }

  // --- Admin: downloads cleanup (eviction sweep) --------------------------

  async getCleanup(signal?: AbortSignal): Promise<CleanupStatus> {
    const { data, error } = await this.api.GET('/admin/cleanup', {
      params: { query: this.q() },
      signal,
    });
    if (error || !data) throw new ImmerleApiError(0, 'cleanup_failed');
    return {
      enabled: data.enabled ?? false,
      intervalSeconds: data.intervalSeconds ?? 0,
      maxAgeSeconds: data.maxAgeSeconds ?? 0,
    };
  }

  async setCleanupEnabled(enabled: boolean): Promise<CleanupStatus> {
    const { data, error } = await this.api.POST('/admin/cleanup', {
      params: { query: { ...this.q(), enabled } },
    });
    if (error || !data) throw new ImmerleApiError(0, 'cleanup_toggle_failed');
    return {
      enabled: data.enabled ?? false,
      intervalSeconds: data.intervalSeconds ?? 0,
      maxAgeSeconds: data.maxAgeSeconds ?? 0,
    };
  }

  /** Run an eviction sweep now; returns the number of removed downloads. */
  async runCleanup(): Promise<number> {
    const { data, error } = await this.api.POST('/admin/cleanup/run', {
      params: { query: this.q() },
    });
    if (error || !data) throw new ImmerleApiError(0, 'cleanup_run_failed');
    return data.removed ?? 0;
  }

  // --- Admin: on-demand catalog (download jobs, legacy) -------------------

  async getDownloadJobs(signal?: AbortSignal): Promise<DownloadJob[]> {
    return this.request<DownloadJob[]>('GET', 'admin/jobs', undefined, signal);
  }

  async retryDownloadJob(id: string): Promise<DownloadJob> {
    return this.request<DownloadJob>('POST', `admin/jobs/${id}/retry`);
  }

  async cancelDownloadJob(id: string): Promise<void> {
    await this.request<void>('POST', `admin/jobs/${id}/cancel`);
  }

  async purgeCache(): Promise<{ freedBytes: number }> {
    return this.request<{ freedBytes: number }>('POST', 'admin/cache/purge');
  }

  /** Request a track be fetched on demand (used by W5 search-and-download). */
  async requestDownload(providerId: string, query: string): Promise<DownloadJob> {
    return this.request<DownloadJob>('POST', 'catalog/download', { providerId, query });
  }

  // --- Admin: federation ---------------------------------------------------

  async getFederationState(signal?: AbortSignal): Promise<FederationState> {
    return this.request<FederationState>('GET', 'admin/federation', undefined, signal);
  }

  async setFederationEnabled(enabled: boolean, hubUrl?: string): Promise<FederationState> {
    return this.request<FederationState>('PUT', 'admin/federation', { enabled, hubUrl });
  }

  async setAnonymizedExport(enabled: boolean): Promise<FederationState> {
    return this.request<FederationState>('PUT', 'admin/federation/export', { enabled });
  }

  // --- Admin: server / transcoding ----------------------------------------

  async getTranscodeProfiles(signal?: AbortSignal): Promise<TranscodeProfile[]> {
    return this.request<TranscodeProfile[]>(
      'GET',
      'admin/transcode-profiles',
      undefined,
      signal,
    );
  }

  async upsertTranscodeProfile(profile: Partial<TranscodeProfile>): Promise<TranscodeProfile> {
    return this.request<TranscodeProfile>('PUT', 'admin/transcode-profiles', profile);
  }

  async getServerSettings(signal?: AbortSignal): Promise<ServerSettings> {
    return this.request<ServerSettings>('GET', 'admin/settings', undefined, signal);
  }

  async updateServerSettings(settings: Partial<ServerSettings>): Promise<ServerSettings> {
    return this.request<ServerSettings>('PUT', 'admin/settings', settings);
  }

  // --- Social: friends & activity (extension API, typed via OpenAPI) -------

  async getFriends(signal?: AbortSignal): Promise<FriendDTO[]> {
    const { data, error } = await this.api.GET('/friends', { params: { query: this.q() }, signal });
    if (error || !data) throw new ImmerleApiError(0, 'friends_failed');
    return data.friends ?? [];
  }

  async getPendingFriends(signal?: AbortSignal): Promise<PendingFriendDTO[]> {
    const { data, error } = await this.api.GET('/friends/pending', {
      params: { query: this.q() },
      signal,
    });
    if (error || !data) throw new ImmerleApiError(0, 'pending_failed');
    return data.pending ?? [];
  }

  async requestFriend(username: string): Promise<void> {
    const { error } = await this.api.POST('/friends/request', {
      params: { query: { ...this.q(), username } },
    });
    if (error) throw new ImmerleApiError(0, 'friend_request_failed');
  }

  async acceptFriend(username: string): Promise<void> {
    const { error } = await this.api.POST('/friends/accept', {
      params: { query: { ...this.q(), username } },
    });
    if (error) throw new ImmerleApiError(0, 'friend_accept_failed');
  }

  async getActivity(signal?: AbortSignal): Promise<ActivityEventDTO[]> {
    const { data, error } = await this.api.GET('/activity', { params: { query: this.q() }, signal });
    if (error || !data) throw new ImmerleApiError(0, 'activity_failed');
    return data.events ?? [];
  }

  // --- Own account (self-service display name + email) ---------------------

  async getAccount(signal?: AbortSignal): Promise<Account> {
    const { data, error } = await this.api.GET('/account', { params: { query: this.q() }, signal });
    if (error || !data?.user) throw new ImmerleApiError(0, 'account_failed');
    return {
      username: data.user.username ?? this.username,
      displayName: data.user.displayName ?? '',
      email: data.user.email ?? '',
      isAdmin: data.user.isAdmin ?? false,
    };
  }

  /** Update the caller's own display name / email (partial). */
  async updateAccount(patch: { displayName?: string; email?: string }): Promise<Account> {
    const { data, error } = await this.api.POST('/account', {
      params: { query: { ...this.q(), ...patch } },
    });
    if (error || !data?.user) throw new ImmerleApiError(0, 'account_update_failed');
    this.setDisplayName(data.user.displayName);
    return {
      username: data.user.username ?? this.username,
      displayName: data.user.displayName ?? '',
      email: data.user.email ?? '',
      isAdmin: data.user.isAdmin ?? false,
    };
  }

  /** A user's profile: identity, recent activity and public playlists. Defaults
   * to the caller when `username` is omitted. */
  async getProfile(username?: string, signal?: AbortSignal): Promise<ProfileResult> {
    const { data, error } = await this.api.GET('/profile', {
      params: { query: { ...this.q(), ...(username ? { username } : {}) } },
      signal,
    });
    if (error || !data) throw new ImmerleApiError(0, 'profile_failed');
    return {
      user: data.user ?? {},
      isSelf: data.isSelf ?? false,
      isFriend: data.isFriend ?? false,
      activity: data.activity ?? [],
      playlists: data.playlists ?? [],
    };
  }

  // --- Jam (real-time listening sessions) ---------------------------------

  async jamCreate(name?: string): Promise<JamResult> {
    const { data, error } = await this.api.POST('/jam/create', {
      params: { query: { ...this.q(), name } },
    });
    if (error || !data) throw new ImmerleApiError(0, 'jam_create_failed');
    return { session: data.session, participants: data.participants ?? [] };
  }

  async jamJoin(sessionId: string): Promise<JamResult> {
    const { data, error } = await this.api.POST('/jam/join', {
      params: { query: { ...this.q(), sessionId } },
    });
    if (error || !data) throw new ImmerleApiError(0, 'jam_join_failed');
    return { session: data.session, participants: data.participants ?? [] };
  }

  async jamState(sessionId: string, signal?: AbortSignal): Promise<JamResult> {
    const { data, error } = await this.api.GET('/jam/state', {
      params: { query: { ...this.q(), sessionId } },
      signal,
    });
    if (error || !data) throw new ImmerleApiError(0, 'jam_state_failed');
    return { session: data.session, participants: data.participants ?? [] };
  }

  /** Host-only. `position` is in milliseconds; `trackIds` is comma-joined. */
  async jamUpdate(
    sessionId: string,
    fields: { currentTrackId?: string; position?: number; state?: string; trackIds?: string },
  ): Promise<JamResult> {
    const { data, error } = await this.api.POST('/jam/update', {
      params: { query: { ...this.q(), sessionId, ...fields } },
    });
    if (error || !data) throw new ImmerleApiError(0, 'jam_update_failed');
    return { session: data.session, participants: data.participants ?? [] };
  }

  async jamLeave(sessionId: string): Promise<void> {
    const { error } = await this.api.POST('/jam/leave', {
      params: { query: { ...this.q(), sessionId } },
    });
    if (error) throw new ImmerleApiError(0, 'jam_leave_failed');
  }

  /** SSE endpoint URL for live Jam events (consumable via EventSource on web). */
  jamEventsUrl(sessionId: string): string {
    return this.subsonic.authedUrl('jam/events', { sessionId });
  }

  // --- Collaborative playlists --------------------------------------------

  async addPlaylistCollaborator(playlistId: string, username: string): Promise<void> {
    const { error } = await this.api.POST('/playlists/collaborators', {
      params: { query: { ...this.q(), playlistId, username } },
    });
    if (error) throw new ImmerleApiError(0, 'collaborator_add_failed');
  }

  // --- Public playlists (discovery + opt-in subscription) -----------------

  /** Browse public playlists; each carries a `subscribed` flag for the caller. */
  async getPublicPlaylists(signal?: AbortSignal): Promise<PublicPlaylistDTO[]> {
    const { data, error } = await this.api.GET('/playlists/public', {
      params: { query: this.q() },
      signal,
    });
    if (error || !data) throw new ImmerleApiError(0, 'public_playlists_failed');
    return data.playlists ?? [];
  }

  async subscribePlaylist(playlistId: string): Promise<void> {
    const { error } = await this.api.POST('/playlists/subscribe', {
      params: { query: { ...this.q(), playlistId } },
    });
    if (error) throw new ImmerleApiError(0, 'subscribe_failed');
  }

  async unsubscribePlaylist(playlistId: string): Promise<void> {
    const { error } = await this.api.POST('/playlists/unsubscribe', {
      params: { query: { ...this.q(), playlistId } },
    });
    if (error) throw new ImmerleApiError(0, 'unsubscribe_failed');
  }

  // --- Playlist imports (external platforms) ------------------------------

  /** Available import sources and whether each is configured server-side. */
  async listImportSources(signal?: AbortSignal): Promise<ImportSourceDTO[]> {
    const { data, error } = await this.api.GET('/imports/sources', { params: { query: this.q() }, signal });
    if (error || !data) throw new ImmerleApiError(0, 'import_sources_failed');
    return data.sources ?? [];
  }

  /** The caller's playlist imports, most recent first (no per-track items). */
  async listImports(signal?: AbortSignal): Promise<ImportDTO[]> {
    const { data, error } = await this.api.GET('/imports', { params: { query: this.q() }, signal });
    if (error || !data) throw new ImmerleApiError(0, 'imports_failed');
    return data.imports ?? [];
  }

  /** Queue an import of an external playlist by `source` + `ref` (id or URL). */
  async startImport(source: string, ref: string): Promise<ImportDTO> {
    const { data, error } = await this.api.POST('/imports/start', {
      params: { query: { ...this.q(), source, ref } },
    });
    if (error || !data?.import) throw new ImmerleApiError(0, 'import_start_failed');
    return data.import;
  }

  /**
   * Validate or modify a flagged import item. With no `query`, validates the
   * flagged candidate as-is; with a `query` ("artist title"), re-searches the
   * providers and uses the best result. Flips the item to "matched".
   */
  async resolveImportItem(itemId: string, query?: string): Promise<ImportItemDTO> {
    const { data, error } = await this.api.POST('/imports/items/resolve', {
      params: { query: { ...this.q(), itemId, ...(query ? { query } : {}) } },
    });
    if (error || !data?.item) throw new ImmerleApiError(0, 'import_resolve_failed');
    return data.item;
  }

  /** One import with its per-track items, for a progress view. */
  async getImportStatus(id: string, signal?: AbortSignal): Promise<ImportDTO> {
    const { data, error } = await this.api.GET('/imports/status', {
      params: { query: { ...this.q(), id } },
      signal,
    });
    if (error || !data?.import) throw new ImmerleApiError(0, 'import_status_failed');
    return data.import;
  }

  // --- Personal API tokens -------------------------------------------------

  async listTokens(signal?: AbortSignal): Promise<APITokenDTO[]> {
    const { data, error } = await this.api.GET('/tokens', { params: { query: this.q() }, signal });
    if (error || !data) throw new ImmerleApiError(0, 'tokens_failed');
    return data.tokens ?? [];
  }

  /** Create a token. The secret is returned ONCE in `token`. */
  async createToken(name?: string, expires?: number): Promise<CreateTokenResponse> {
    const { data, error } = await this.api.POST('/tokens/create', {
      params: { query: { ...this.q(), name, expires } },
    });
    if (error || !data) throw new ImmerleApiError(0, 'token_create_failed');
    return data;
  }

  async revokeToken(id: string): Promise<void> {
    const { error } = await this.api.POST('/tokens/revoke', {
      params: { query: { ...this.q(), id } },
    });
    if (error) throw new ImmerleApiError(0, 'token_revoke_failed');
  }

  // --- Per-account UI theme ------------------------------------------------

  /** The caller's stored theme (accent colour, etc.). */
  async getTheme(signal?: AbortSignal): Promise<ThemeDTO> {
    const { data, error } = await this.api.GET('/theme', { params: { query: this.q() }, signal });
    if (error || !data) throw new ImmerleApiError(0, 'theme_failed');
    return data.theme ?? {};
  }

  /**
   * Persist the caller's accent colour. Pass a CSS hex (e.g. `#3b82f6`), or an
   * empty string to clear it (server falls back to the client default).
   */
  async setTheme(accentColor: string): Promise<ThemeDTO> {
    const { data, error } = await this.api.POST('/theme', {
      params: { query: { ...this.q(), accentColor } },
    });
    if (error || !data) throw new ImmerleApiError(0, 'theme_update_failed');
    return data.theme ?? {};
  }
}

/** Jam session, with the server-sent timestamps used for drift-corrected sync. */
export type JamSession = JamSessionDTO & { updatedAt?: string; createdAt?: string };

/** Normalized result of a Jam call: session plus its participants. */
export interface JamResult {
  session?: JamSession;
  participants: JamParticipantDTO[];
}

/** Raw `/admin/settings` response body. */
interface SettingsResponseRaw {
  ok?: boolean;
  settings?: RuntimeSettingsDTO;
  restartRequired?: boolean;
  pendingRestart?: string[];
}

/** Runtime settings plus the pending-restart status returned by the server. */
export interface SettingsResult {
  settings: RuntimeSettingsDTO;
  restartRequired: boolean;
  /** Sub-systems whose change only takes effect after a restart. */
  pendingRestart: string[];
}

/** Downloads eviction-sweep status. */
export interface CleanupStatus {
  enabled: boolean;
  intervalSeconds: number;
  maxAgeSeconds: number;
}

/** The caller's own account, editable via `/account`. */
export interface Account {
  username: string;
  displayName: string;
  email: string;
  isAdmin: boolean;
}

/** A user's profile: identity, recent activity and public playlists. */
export interface ProfileResult {
  user: NonNullable<ProfileResponse['user']>;
  isSelf: boolean;
  isFriend: boolean;
  activity: ActivityEventDTO[];
  playlists: ProfilePlaylistDTO[];
}
