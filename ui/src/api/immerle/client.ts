import { SubsonicClient } from '../subsonic/client';
import {
  Album,
  AlbumWithSongs,
  Artist,
  ArtistWithAlbums,
  Genre,
  NowPlayingEntry,
  Playlist,
  PlaylistWithSongs,
  SearchResult3,
  Song,
} from '../subsonic/types';
import {
  Starred,
  toAlbum,
  toAlbumWithSongs,
  toArtist,
  toArtistWithAlbums,
  toGenre,
  toNowPlaying,
  toPlaylist,
  toPlaylistWithSongs,
  toSearchResult,
  toSong,
  toStarred,
} from './catalog';
import {
  ActivityEventDTO,
  APITokenDTO,
  ApiError,
  ErrorResponse,
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
  ProviderLog,
  ScanProgress,
  RadioStation,
  ServerSettings,
  SmartPlaylist,
  SmartRules,
  TrackEdit,
  TranscodeProfile,
  Wrapped,
} from './types';

/**
 * Build an {@link ImmerleApiError} from an openapi-fetch error body, preferring
 * the server's `code`/`params` (for i18n) and falling back to a local code.
 */
function apiErr(error: ErrorResponse | undefined, fallbackCode: string): ImmerleApiError {
  const e = error?.error;
  return new ImmerleApiError(0, e?.message ?? fallbackCode, e?.code ?? fallbackCode, e?.params);
}

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
    version: dto.version ?? undefined,
  };
}

/**
 * Capability-aware client that composes the raw Subsonic surface with the
 * Immerle REST API (mounted under `/api/v1`).
 *
 * Standard music operations are delegated to the embedded {@link SubsonicClient}
 * (`api.subsonic.*`). Immerle-specific operations live here and are guarded by
 * {@link has} so the UI can hide what the instance does not advertise.
 *
 * Every Immerle REST call is authenticated with the session's Bearer token (a
 * device JWT obtained at login by exchanging the Subsonic salted token).
 */
export class ImmerleClient {
  /** Admin status derived from the Subsonic user's adminRole (set post-construction). */
  private adminFlag = false;
  /** Free-text display name (set post-construction); falls back to the username. */
  private displayNameValue?: string;
  private _api?: ImmerleApi;
  private _apiToken?: string;

  constructor(
    public readonly subsonic: SubsonicClient,
    public readonly capabilities: Capabilities,
    private session: ImmerleSession | null = null,
  ) {}

  /** Authenticated, typed REST client (rebuilt when the session token changes). */
  private get api(): ImmerleApi {
    const token = this.session?.token ?? '';
    if (!this._api || this._apiToken !== token) {
      this._api = createAuthedImmerleApi(this.serverUrl, token);
      this._apiToken = token;
    }
    return this._api;
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
    const clean = path.replace(/^\/+/, '');
    const url = `${this.serverUrl}/api/v1/${clean}`;

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
      let code: string | undefined;
      let params: Record<string, unknown> | undefined;
      try {
        const j = (await res.json()) as { error?: ApiError };
        code = j.error?.code;
        params = j.error?.params;
        message = j.error?.message ?? code ?? message;
      } catch {
        /* ignore non-JSON error bodies */
      }
      throw new ImmerleApiError(res.status, message, code, params);
    }
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  // --- Admin: library ------------------------------------------------------

  /**
   * Catalog browse over the native REST API. The native DTOs are normalized into
   * the app's domain types (see catalog.ts), so these are drop-in replacements
   * for the Subsonic browse methods.
   */
  async getArtists(signal?: AbortSignal): Promise<Artist[]> {
    const { data, error } = await this.api.GET('/artists', { signal });
    if (error) throw apiErr(error, 'browse.artists');
    return (data.artists ?? []).map(toArtist);
  }

  async getArtist(id: string, signal?: AbortSignal): Promise<ArtistWithAlbums> {
    const { data, error } = await this.api.GET('/artists/{id}', {
      params: { path: { id } },
      signal,
    });
    if (error) throw apiErr(error, 'browse.artist');
    return toArtistWithAlbums(data);
  }

  async getAlbum(id: string, signal?: AbortSignal): Promise<AlbumWithSongs> {
    const { data, error } = await this.api.GET('/albums/{id}', { params: { path: { id } }, signal });
    if (error) throw apiErr(error, 'browse.album');
    return toAlbumWithSongs(data);
  }

  async getAlbumList(
    type: string,
    opts?: { size?: number; offset?: number; genre?: string; fromYear?: number; toYear?: number },
    signal?: AbortSignal,
  ): Promise<Album[]> {
    const { data, error } = await this.api.GET('/albums', {
      params: { query: { type, size: opts?.size, offset: opts?.offset, genre: opts?.genre, fromYear: opts?.fromYear, toYear: opts?.toYear } },
      signal,
    });
    if (error) throw apiErr(error, 'browse.albumList');
    return (data.albums ?? []).map(toAlbum);
  }

  async getGenres(signal?: AbortSignal): Promise<Genre[]> {
    const { data, error } = await this.api.GET('/genres', { signal });
    if (error) throw apiErr(error, 'browse.genres');
    return (data.genres ?? []).map(toGenre);
  }

  async getSongsByGenre(genre: string, count = 200, signal?: AbortSignal): Promise<Song[]> {
    const { data, error } = await this.api.GET('/songs', { params: { query: { genre, count } }, signal });
    if (error) throw apiErr(error, 'browse.songsByGenre');
    return (data.songs ?? []).map(toSong);
  }

  async getSong(id: string, signal?: AbortSignal): Promise<Song> {
    const { data, error } = await this.api.GET('/songs/{id}', { params: { path: { id } }, signal });
    if (error) throw apiErr(error, 'browse.song');
    return toSong(data);
  }

  /** The caller's starred catalog (artists/albums/songs). */
  async getStarred(signal?: AbortSignal): Promise<Starred> {
    const { data, error } = await this.api.GET('/me/favorites', { signal });
    if (error) throw apiErr(error, 'browse.favorites');
    return toStarred(data);
  }

  /** Search artists, albums and songs (merging remote-provider results). */
  async search(query: string, signal?: AbortSignal): Promise<SearchResult3> {
    const { data, error } = await this.api.GET('/search', { params: { query: { q: query } }, signal });
    if (error) throw apiErr(error, 'search');
    return toSearchResult(data);
  }

  /**
   * Playlist CRUD over the native REST API, normalized into the app's domain
   * types — drop-in replacements for the Subsonic playlist methods. (Public /
   * collaborative playlist ops live in their own native methods below.)
   */
  async getPlaylists(signal?: AbortSignal): Promise<Playlist[]> {
    const { data, error } = await this.api.GET('/playlists', { signal });
    if (error) throw apiErr(error, 'playlist.list');
    return (data.playlists ?? []).map(toPlaylist);
  }

  async getPlaylist(id: string, signal?: AbortSignal): Promise<PlaylistWithSongs> {
    const { data, error } = await this.api.GET('/playlists/{id}', { params: { path: { id } }, signal });
    if (error) throw apiErr(error, 'playlist.get');
    return toPlaylistWithSongs(data);
  }

  async createPlaylist(name: string, songIds: string[] = []): Promise<void> {
    const { error } = await this.api.POST('/playlists', { body: { name, ids: songIds } });
    if (error) throw apiErr(error, 'playlist.create');
  }

  async updatePlaylist(
    id: string,
    opts: {
      name?: string;
      comment?: string;
      public?: boolean;
      songIdToAdd?: string[];
      songIndexToRemove?: number[];
    },
  ): Promise<void> {
    const { error } = await this.api.PATCH('/playlists/{id}', {
      params: { path: { id } },
      body: {
        name: opts.name,
        comment: opts.comment,
        public: opts.public,
        addIds: opts.songIdToAdd,
        removeIndexes: opts.songIndexToRemove,
      },
    });
    if (error) throw apiErr(error, 'playlist.update');
  }

  async deletePlaylist(id: string): Promise<void> {
    const { error } = await this.api.DELETE('/playlists/{id}', { params: { path: { id } } });
    if (error) throw apiErr(error, 'playlist.delete');
  }

  /**
   * Per-user state (favorites, plays, play queue, now-playing) over the native
   * REST API — drop-in replacements for the Subsonic methods.
   */
  async star(opts: { id?: string; albumId?: string; artistId?: string }): Promise<void> {
    await this.toggleStar(opts, true);
  }

  async unstar(opts: { id?: string; albumId?: string; artistId?: string }): Promise<void> {
    await this.toggleStar(opts, false);
  }

  private async toggleStar(
    opts: { id?: string; albumId?: string; artistId?: string },
    on: boolean,
  ): Promise<void> {
    let res;
    if (opts.id) {
      const p = { params: { path: { id: opts.id } } } as const;
      res = on ? await this.api.PUT('/songs/{id}/star', p) : await this.api.DELETE('/songs/{id}/star', p);
    } else if (opts.albumId) {
      const p = { params: { path: { id: opts.albumId } } } as const;
      res = on ? await this.api.PUT('/albums/{id}/star', p) : await this.api.DELETE('/albums/{id}/star', p);
    } else if (opts.artistId) {
      const p = { params: { path: { id: opts.artistId } } } as const;
      res = on ? await this.api.PUT('/artists/{id}/star', p) : await this.api.DELETE('/artists/{id}/star', p);
    } else {
      return;
    }
    if (res.error) throw apiErr(res.error, 'star');
  }

  async scrobble(id: string, submission: boolean, time?: number): Promise<void> {
    const { error } = await this.api.POST('/scrobbles', { body: { ids: [id], submission, playedAt: time } });
    if (error) throw apiErr(error, 'scrobble');
  }

  async savePlayQueue(songIds: string[], current?: string, positionMs?: number): Promise<void> {
    const { error } = await this.api.PUT('/play-queue', {
      body: { ids: songIds, current, position: positionMs, client: 'immerle' },
    });
    if (error) throw apiErr(error, 'playqueue.save');
  }

  async getNowPlaying(signal?: AbortSignal): Promise<NowPlayingEntry[]> {
    const { data, error } = await this.api.GET('/now-playing', { signal });
    if (error) throw apiErr(error, 'nowplaying');
    return (data.nowPlaying ?? []).map(toNowPlaying);
  }

  /**
   * Public cover-art URL for a track or album id (loadable as a plain <img>, no
   * credential in the URL). Returns undefined when there is no id.
   */
  coverArtUrl(coverArtId: string | undefined, size?: number): string | undefined {
    if (!coverArtId) return undefined;
    const url = `${this.serverUrl}/api/v1/cover/${encodeURIComponent(coverArtId)}`;
    return size ? `${url}?size=${size}` : url;
  }

  /**
   * A playable stream URL for a track: mints a short-lived signed URL (no
   * credential in it, safe for an <audio>/<video> src) and appends the transcode
   * options. The transcode params aren't part of the signature, so they can be
   * tweaked freely. Players mint these per track when building the queue.
   */
  async streamUrl(
    id: string,
    opts?: { maxBitRate?: number; format?: string },
    signal?: AbortSignal,
  ): Promise<string> {
    const { data, error } = await this.api.GET('/songs/{id}/stream-url', { params: { path: { id } }, signal });
    if (error) throw apiErr(error, 'stream.url');
    let url = `${this.serverUrl}${data.stream}`; // signed path: /api/v1/songs/{id}/stream?exp=&sig=
    if (opts?.maxBitRate) url += `&maxBitRate=${opts.maxBitRate}`;
    if (opts?.format) url += `&format=${encodeURIComponent(opts.format)}`;
    return url;
  }

  /**
   * Library-wide stats (counts + on-disk size in bytes) from `/library/stats`.
   * Falls back to deriving counts from Subsonic on a plain server (no size).
   */
  async getLibraryStats(signal?: AbortSignal): Promise<LibraryStats> {
    try {
      const { data, error } = await this.api.GET('/library/stats', { signal });
      if (!error && data) {
        return {
          artistCount: data.artists ?? 0,
          albumCount: data.albums ?? 0,
          songCount: data.tracks ?? 0,
          totalSize: data.totalSize ?? 0,
          lastScan: data.updatedAt,
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

  // --- Admin: library tracks (manage ANY track) ---------------------------

  /** List downloaded tracks (paginated, searchable). Admin-only. */
  async adminListTracks(
    opts: { query?: string; limit?: number; offset?: number } = {},
    signal?: AbortSignal,
  ): Promise<{ tracks: Song[]; total: number; limit: number; offset: number }> {
    const { data, error } = await this.api.GET('/admin/tracks', {
      params: { query: { query: opts.query, limit: opts.limit, offset: opts.offset } },
      signal,
    });
    if (error || !data) throw apiErr(error, 'tracks_failed');
    return {
      tracks: (data.tracks ?? []) as Song[],
      total: data.total ?? 0,
      limit: data.limit ?? 0,
      offset: data.offset ?? 0,
    };
  }

  /** Edit any track's simple metadata (title/genre/year/track/disc). Admin-only. */
  async adminUpdateTrack(id: string, edit: TrackEdit): Promise<Song> {
    const { data, error } = await this.api.PATCH('/admin/tracks/{id}', {
      params: { path: { id } },
      body: edit,
    });
    if (error || !data) throw apiErr(error, 'track_update_failed');
    return data as Song;
  }

  /**
   * Replace any track's cover from a local image. `uri` is a file/content URI
   * (image-picker result); reuses {@link uploadForm} for the multipart plumbing.
   * Admin-only.
   */
  async adminSetTrackCover(id: string, uri: string, mime = 'image/jpeg'): Promise<Song> {
    const form = new FormData();
    const name = `cover.${mime.split('/')[1] ?? 'jpg'}`;
    // React Native FormData accepts {uri,name,type}; web needs a real Blob.
    if (uri.startsWith('data:') || uri.startsWith('blob:') || uri.startsWith('http')) {
      const blob = await (await fetch(uri)).blob();
      form.append('file', blob, name);
    } else {
      form.append('file', { uri, name, type: mime } as unknown as Blob);
    }
    return this.uploadForm<Song>('PUT', `admin/tracks/${encodeURIComponent(id)}/cover`, form);
  }

  /** Delete any track: its file, DB row and references. Admin-only. */
  async adminDeleteTrack(id: string): Promise<void> {
    const { error } = await this.api.DELETE('/admin/tracks/{id}', {
      params: { path: { id } },
    });
    if (error) throw apiErr(error, 'track_delete_failed');
  }

  // --- Admin: dynamic providers (typed via OpenAPI) -----------------------

  /** List configured providers (with live `enabled`/`active` status). Admin-only. */
  async listProviders(signal?: AbortSignal): Promise<Provider[]> {
    const { data, error } = await this.api.GET('/admin/providers', { signal });
    if (error || !data) throw apiErr(error, 'providers_failed');
    return data.map(toProvider);
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
    const { error } = await this.api.POST('/admin/providers', {
      body: {
        name: p.name,
        endpoint: p.endpoint,
        config: p.config,
        enabled: p.enabled,
        kind: p.kind,
      },
    });
    if (error) throw apiErr(error, 'provider_upsert_failed');
    return this.listProviders();
  }

  /** Create a dynamic HTTP provider from just its URL. The server probes the
   * remote's /capabilities to derive the name and seed the config skeleton; the
   * provider is created disabled. Returns the refreshed list. */
  async createProvider(endpoint: string): Promise<Provider[]> {
    const { error } = await this.api.POST('/admin/providers', { body: { endpoint } });
    if (error) throw apiErr(error, 'provider_create_failed');
    return this.listProviders();
  }

  /** Toggle a provider on/off; applied to the live registry immediately. */
  async setProviderEnabled(name: string, enabled: boolean): Promise<Provider> {
    const { data, error } = await this.api.PUT('/admin/providers/{name}/enabled', {
      params: { path: { name } },
      body: { enabled },
    });
    if (error || !data) throw apiErr(error, 'provider_enable_failed');
    return toProvider(data);
  }

  /** Delete a provider config and unregister it. Built-ins are not deletable. */
  async deleteProvider(name: string): Promise<void> {
    const { error } = await this.api.DELETE('/admin/providers/{name}', {
      params: { path: { name } },
    });
    if (error) throw apiErr(error, 'provider_delete_failed');
  }

  /**
   * Set the provider priority order (lower = higher priority). `order` must list
   * every provider name exactly once. Order also drives the search fallback.
   */
  async reorderProviders(order: string[]): Promise<Provider[]> {
    const { data, error } = await this.api.PUT('/admin/providers/order', {
      body: { order },
    });
    if (error || !data) throw apiErr(error, 'provider_reorder_failed');
    return data.map(toProvider);
  }

  /** Recent warn/error events for a provider (newest first). Admin-only. */
  async getProviderLogs(name: string, signal?: AbortSignal): Promise<ProviderLog[]> {
    return this.request<ProviderLog[]>('GET', `admin/providers/${encodeURIComponent(name)}/logs`, undefined, signal);
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

  /** Apply a partial settings update (send only the sub-objects that changed). */
  async updateSettings(patch: RuntimeSettingsDTO): Promise<SettingsResult> {
    const data = await this.request<SettingsResponseRaw>('PATCH', 'admin/settings', patch);
    return {
      settings: data.settings ?? {},
      restartRequired: data.restartRequired ?? false,
      pendingRestart: data.pendingRestart ?? [],
    };
  }

  // --- Smart playlists (rule-based) ---------------------------------------

  async listSmartPlaylists(signal?: AbortSignal): Promise<SmartPlaylist[]> {
    const r = await this.request<{ playlists?: SmartPlaylist[] }>('GET', 'smart-playlists', undefined, signal);
    return r.playlists ?? [];
  }

  async createSmartPlaylist(name: string, rules: SmartRules): Promise<SmartPlaylist> {
    return this.request<SmartPlaylist>('POST', 'smart-playlists', { name, rules });
  }

  async updateSmartPlaylist(id: string, name: string, rules: SmartRules): Promise<SmartPlaylist> {
    return this.request<SmartPlaylist>('PUT', `smart-playlists/${id}`, { name, rules });
  }

  async deleteSmartPlaylist(id: string): Promise<void> {
    await this.request<void>('DELETE', `smart-playlists/${id}`);
  }

  /** Resolve a saved smart playlist to its current tracks (ready to enqueue). */
  async getSmartPlaylistTracks(id: string, signal?: AbortSignal): Promise<Song[]> {
    const r = await this.request<{ songs?: Song[] }>('GET', `smart-playlists/${id}/tracks`, undefined, signal);
    return r.songs ?? [];
  }

  /** Preview ad-hoc rules without saving (for the editor). */
  async previewSmartPlaylist(rules: SmartRules, signal?: AbortSignal): Promise<Song[]> {
    const r = await this.request<{ songs?: Song[] }>('POST', 'smart-playlists/preview', { rules }, signal);
    return r.songs ?? [];
  }

  async getSmartPlaylistsEnabled(signal?: AbortSignal): Promise<boolean> {
    const r = await this.request<{ enabled: boolean }>('GET', 'admin/smart-playlists', undefined, signal);
    return !!r.enabled;
  }

  async setSmartPlaylistsEnabled(enabled: boolean): Promise<boolean> {
    const r = await this.request<{ enabled: boolean }>('PUT', 'admin/smart-playlists', { enabled });
    return !!r.enabled;
  }

  // --- Internet radio -----------------------------------------------------

  async listRadioStations(signal?: AbortSignal): Promise<RadioStation[]> {
    const r = await this.request<{ stations?: RadioStation[] }>('GET', 'radio', undefined, signal);
    return r.stations ?? [];
  }

  /** Public URL of a station's locally-cached logo (no auth needed). The id can
   * contain ':' (e.g. "builtin:nrj"), which is a valid path char and is sent raw
   * — matching the admin endpoints (the server also tolerates a %3A-encoded id). */
  radioCoverUrl(id: string): string {
    return `${this.serverUrl}/api/v1/radio/stations/${id}/cover`;
  }

  async createRadioStation(body: { name: string; streamUrl: string; homepageUrl?: string; coverUrl?: string }): Promise<RadioStation> {
    return this.request<RadioStation>('POST', 'admin/radio/stations', body);
  }

  async updateRadioStation(id: string, body: { name: string; streamUrl: string; homepageUrl?: string; coverUrl?: string }): Promise<RadioStation> {
    return this.request<RadioStation>('PUT', `admin/radio/stations/${id}`, body);
  }

  async deleteRadioStation(id: string): Promise<void> {
    await this.request<void>('DELETE', `admin/radio/stations/${id}`);
  }

  /** Favorite / unfavorite a station (kept separate from track stars). */
  async setRadioLiked(id: string, liked: boolean): Promise<void> {
    await this.request<void>(liked ? 'PUT' : 'DELETE', `radio/stations/${id}/like`);
  }

  async getRadioEnabled(signal?: AbortSignal): Promise<boolean> {
    const r = await this.request<{ enabled: boolean }>('GET', 'admin/radio', undefined, signal);
    return !!r.enabled;
  }

  async setRadioEnabled(enabled: boolean): Promise<boolean> {
    const r = await this.request<{ enabled: boolean }>('PUT', 'admin/radio', { enabled });
    return !!r.enabled;
  }

  // --- Wrapped (year-in-review) -------------------------------------------

  /** The caller's year-in-review (defaults to the current year server-side). */
  async getWrapped(year?: number, signal?: AbortSignal): Promise<Wrapped> {
    const path = year ? `wrapped?year=${year}` : 'wrapped';
    return this.request<Wrapped>('GET', path, undefined, signal);
  }

  /** Admin: whether the Wrapped feature is enabled. */
  async getWrappedEnabled(signal?: AbortSignal): Promise<boolean> {
    const r = await this.request<{ enabled: boolean }>('GET', 'admin/wrapped', undefined, signal);
    return !!r.enabled;
  }

  /** Admin: turn the Wrapped feature on or off. */
  async setWrappedEnabled(enabled: boolean): Promise<boolean> {
    const r = await this.request<{ enabled: boolean }>('PUT', 'admin/wrapped', { enabled });
    return !!r.enabled;
  }

  // --- Admin: downloads cleanup (eviction sweep) --------------------------

  async getCleanup(signal?: AbortSignal): Promise<CleanupStatus> {
    const { data, error } = await this.api.GET('/admin/cleanup', { signal });
    if (error || !data) throw apiErr(error, 'cleanup_failed');
    return {
      enabled: data.enabled ?? false,
      intervalSeconds: data.intervalSeconds ?? 0,
      maxAgeSeconds: data.maxAgeSeconds ?? 0,
    };
  }

  async setCleanupEnabled(enabled: boolean): Promise<CleanupStatus> {
    const { data, error } = await this.api.PUT('/admin/cleanup', { body: { enabled } });
    if (error || !data) throw apiErr(error, 'cleanup_toggle_failed');
    return {
      enabled: data.enabled ?? false,
      intervalSeconds: data.intervalSeconds ?? 0,
      maxAgeSeconds: data.maxAgeSeconds ?? 0,
    };
  }

  /** Run an eviction sweep now; returns the number of removed downloads. */
  async runCleanup(): Promise<number> {
    const { data, error } = await this.api.POST('/admin/cleanup/runs', {});
    if (error || !data) throw apiErr(error, 'cleanup_run_failed');
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
    return this.request<ServerSettings>('PATCH', 'admin/settings', settings);
  }

  // --- Social: friends & activity (typed via OpenAPI) ----------------------

  async getFriends(signal?: AbortSignal): Promise<FriendDTO[]> {
    const { data, error } = await this.api.GET('/friends', { signal });
    if (error || !data) throw apiErr(error, 'friends_failed');
    return data;
  }

  async getPendingFriends(signal?: AbortSignal): Promise<PendingFriendDTO[]> {
    const { data, error } = await this.api.GET('/friends/requests', { signal });
    if (error || !data) throw apiErr(error, 'pending_failed');
    return data;
  }

  async requestFriend(username: string): Promise<void> {
    const { error } = await this.api.POST('/friends/requests', { body: { username } });
    if (error) throw apiErr(error, 'friend_request_failed');
  }

  async acceptFriend(username: string): Promise<void> {
    const { error } = await this.api.POST('/friends/requests/{username}/accept', {
      params: { path: { username } },
    });
    if (error) throw apiErr(error, 'friend_accept_failed');
  }

  async getActivity(signal?: AbortSignal): Promise<ActivityEventDTO[]> {
    const { data, error } = await this.api.GET('/activity', { signal });
    if (error || !data) throw apiErr(error, 'activity_failed');
    return data;
  }

  // --- Own account (self-service display name + email) ---------------------

  async getAccount(signal?: AbortSignal): Promise<Account> {
    const { data, error } = await this.api.GET('/me', { signal });
    if (error || !data) throw apiErr(error, 'account_failed');
    return {
      username: data.username ?? this.username,
      displayName: data.displayName ?? '',
      email: data.email ?? '',
      isAdmin: data.isAdmin ?? false,
      language: (data.language ?? '') as AccountLanguage,
    };
  }

  /** Update the caller's own display name / email / language (partial). */
  async updateAccount(patch: { displayName?: string; email?: string; language?: AccountLanguage }): Promise<Account> {
    const { data, error } = await this.api.PATCH('/me', { body: patch });
    if (error || !data) throw apiErr(error, 'account_update_failed');
    this.setDisplayName(data.displayName);
    return {
      username: data.username ?? this.username,
      displayName: data.displayName ?? '',
      email: data.email ?? '',
      isAdmin: data.isAdmin ?? false,
      language: (data.language ?? '') as AccountLanguage,
    };
  }

  /** A user's profile: identity, recent activity and public playlists. Defaults
   * to the caller when `username` is omitted. */
  async getProfile(username?: string, signal?: AbortSignal): Promise<ProfileResult> {
    const { data, error } = await this.api.GET('/users/{username}', {
      params: { path: { username: username ?? 'me' } },
      signal,
    });
    if (error || !data) throw apiErr(error, 'profile_failed');
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
    const { data, error } = await this.api.POST('/jam', { body: { name } });
    if (error || !data) throw apiErr(error, 'jam_create_failed');
    return { session: data.session, participants: data.participants ?? [] };
  }

  async jamJoin(sessionId: string): Promise<JamResult> {
    const { data, error } = await this.api.POST('/jam/{id}/participants', {
      params: { path: { id: sessionId } },
    });
    if (error || !data) throw apiErr(error, 'jam_join_failed');
    return { session: data.session, participants: data.participants ?? [] };
  }

  async jamState(sessionId: string, signal?: AbortSignal): Promise<JamResult> {
    const { data, error } = await this.api.GET('/jam/{id}', {
      params: { path: { id: sessionId } },
      signal,
    });
    if (error || !data) throw apiErr(error, 'jam_state_failed');
    return { session: data.session, participants: data.participants ?? [] };
  }

  /** Host-only. `position` is in milliseconds; `trackIds` is a track-id list. */
  async jamUpdate(
    sessionId: string,
    fields: { currentTrackId?: string; position?: number; state?: string; trackIds?: string[] },
  ): Promise<JamResult> {
    const { data, error } = await this.api.PATCH('/jam/{id}', {
      params: { path: { id: sessionId } },
      body: fields,
    });
    if (error || !data) throw apiErr(error, 'jam_update_failed');
    return { session: data.session, participants: data.participants ?? [] };
  }

  async jamLeave(sessionId: string): Promise<void> {
    const { error } = await this.api.DELETE('/jam/{id}/participants/me', {
      params: { path: { id: sessionId } },
    });
    if (error) throw apiErr(error, 'jam_leave_failed');
  }

  /** Host-only. Ends the session and removes all participants. */
  async jamEnd(sessionId: string): Promise<void> {
    const { error } = await this.api.DELETE('/jam/{id}', {
      params: { path: { id: sessionId } },
    });
    if (error) throw apiErr(error, 'jam_end_failed');
  }

  /** SSE endpoint URL for live Jam events. EventSource can't set headers, so the
   * Bearer token is passed via the `apiKey` query fallback. */
  jamEventsUrl(sessionId: string): string {
    const token = this.session?.token ?? '';
    return `${this.serverUrl}/api/v1/jam/${sessionId}/events?apiKey=${encodeURIComponent(token)}`;
  }

  // --- Collaborative playlists --------------------------------------------

  async addPlaylistCollaborator(playlistId: string, username: string): Promise<void> {
    const { error } = await this.api.POST('/playlists/{id}/collaborators', {
      params: { path: { id: playlistId } },
      body: { username },
    });
    if (error) throw apiErr(error, 'collaborator_add_failed');
  }

  // --- Public playlists (discovery + opt-in subscription) -----------------

  /** Browse public playlists; each carries a `subscribed` flag for the caller. */
  async getPublicPlaylists(signal?: AbortSignal): Promise<PublicPlaylistDTO[]> {
    const { data, error } = await this.api.GET('/playlists/public', { signal });
    if (error || !data) throw apiErr(error, 'public_playlists_failed');
    return data;
  }

  async subscribePlaylist(playlistId: string): Promise<void> {
    const { error } = await this.api.PUT('/playlists/{id}/subscription', {
      params: { path: { id: playlistId } },
    });
    if (error) throw apiErr(error, 'subscribe_failed');
  }

  async unsubscribePlaylist(playlistId: string): Promise<void> {
    const { error } = await this.api.DELETE('/playlists/{id}/subscription', {
      params: { path: { id: playlistId } },
    });
    if (error) throw apiErr(error, 'unsubscribe_failed');
  }

  // --- Playlist imports (external platforms) ------------------------------

  /** Available import sources and whether each is configured server-side. */
  async listImportSources(signal?: AbortSignal): Promise<ImportSourceDTO[]> {
    const { data, error } = await this.api.GET('/imports/sources', { signal });
    if (error || !data) throw apiErr(error, 'import_sources_failed');
    return data;
  }

  /** The caller's playlist imports, most recent first (no per-track items). */
  async listImports(signal?: AbortSignal): Promise<ImportDTO[]> {
    const { data, error } = await this.api.GET('/imports', { signal });
    if (error || !data) throw apiErr(error, 'imports_failed');
    return data;
  }

  /** Queue an import of an external playlist by `source` + `ref` (id or URL). */
  async startImport(source: string, ref: string): Promise<ImportDTO> {
    const { data, error } = await this.api.POST('/imports', { body: { source, ref } });
    if (error || !data) throw apiErr(error, 'import_start_failed');
    return data;
  }

  /**
   * Validate or modify a flagged import item. With no `query`, validates the
   * flagged candidate as-is; with a `query` ("artist title"), re-searches the
   * providers and uses the best result. Flips the item to "matched".
   */
  async resolveImportItem(importId: string, itemId: string, query?: string): Promise<ImportItemDTO> {
    const { data, error } = await this.api.POST('/imports/{id}/items/{itemId}/resolve', {
      params: { path: { id: importId, itemId } },
      body: query ? { query } : {},
    });
    if (error || !data) throw apiErr(error, 'import_resolve_failed');
    return data;
  }

  /** One import with its per-track items, for a progress view. */
  async getImportStatus(id: string, signal?: AbortSignal): Promise<ImportDTO> {
    const { data, error } = await this.api.GET('/imports/{id}', {
      params: { path: { id } },
      signal,
    });
    if (error || !data) throw apiErr(error, 'import_status_failed');
    return data;
  }

  // --- Personal API tokens -------------------------------------------------

  async listTokens(signal?: AbortSignal): Promise<APITokenDTO[]> {
    const { data, error } = await this.api.GET('/tokens', { signal });
    if (error || !data) throw apiErr(error, 'tokens_failed');
    return data;
  }

  /** Create a token. The secret is returned ONCE in `token`. `expiresAt` is an
   * optional RFC3339 timestamp. */
  async createToken(name?: string, expiresAt?: string): Promise<CreateTokenResponse> {
    const { data, error } = await this.api.POST('/tokens', { body: { name, expiresAt } });
    if (error || !data) throw apiErr(error, 'token_create_failed');
    return data;
  }

  async revokeToken(id: string): Promise<void> {
    const { error } = await this.api.DELETE('/tokens/{id}', { params: { path: { id } } });
    if (error) throw apiErr(error, 'token_revoke_failed');
  }

  // --- Per-account UI theme ------------------------------------------------

  /** The caller's stored theme (accent colour, etc.). */
  async getTheme(signal?: AbortSignal): Promise<ThemeDTO> {
    const { data, error } = await this.api.GET('/theme', { signal });
    if (error || !data) throw apiErr(error, 'theme_failed');
    return data;
  }

  /**
   * Persist the caller's accent colour. Pass a CSS hex (e.g. `#3b82f6`), or an
   * empty string to clear it (server falls back to the client default).
   */
  async setTheme(accentColor: string): Promise<ThemeDTO> {
    const { data, error } = await this.api.PATCH('/theme', { body: { accentColor } });
    if (error || !data) throw apiErr(error, 'theme_update_failed');
    return data;
  }

  // --- "Local" library: user-uploaded tracks -----------------------------

  /** The tracks the caller uploaded ("local" virtual playlist), newest first. */
  async listLocalSongs(signal?: AbortSignal): Promise<Song[]> {
    const data = await this.request<{ songs?: Song[] }>('GET', 'library/local', undefined, signal);
    return data.songs ?? [];
  }

  /** Upload an audio file; returns the ingested track. Web: pass a File. */
  async uploadTrack(file: File, signal?: AbortSignal): Promise<Song> {
    const form = new FormData();
    form.append('file', file, file.name);
    return this.uploadForm<Song>('POST', 'library/uploads', form, signal);
  }

  /** Rename one of the caller's uploaded tracks. */
  async renameTrack(id: string, title: string): Promise<Song> {
    return this.request<Song>('PATCH', `library/tracks/${encodeURIComponent(id)}`, { title });
  }

  /** Replace the cover art of one of the caller's uploaded tracks. */
  async setTrackCover(id: string, image: File): Promise<Song> {
    const form = new FormData();
    form.append('file', image, image.name);
    return this.uploadForm<Song>('PUT', `library/tracks/${encodeURIComponent(id)}/cover`, form);
  }

  /** Delete one of the caller's uploaded tracks (catalog row + file). */
  async deleteTrack(id: string): Promise<void> {
    await this.request<void>('DELETE', `library/tracks/${encodeURIComponent(id)}`);
  }

  // Multipart sibling of `request`: lets the browser set the multipart boundary
  // (so we must NOT set Content-Type ourselves).
  private async uploadForm<T>(method: string, path: string, form: FormData, signal?: AbortSignal): Promise<T> {
    const url = `${this.serverUrl}/api/v1/${path.replace(/^\/+/, '')}`;
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (this.session?.token) headers.Authorization = `Bearer ${this.session.token}`;
    const res = await fetch(url, { method, headers, body: form, signal });
    if (!res.ok) {
      let message = `HTTP ${res.status}`;
      let code: string | undefined;
      let params: Record<string, unknown> | undefined;
      try {
        const j = (await res.json()) as { error?: ApiError };
        code = j.error?.code;
        params = j.error?.params;
        message = j.error?.message ?? code ?? message;
      } catch {
        /* ignore non-JSON error bodies */
      }
      throw new ImmerleApiError(res.status, message, code, params);
    }
    return (await res.json()) as T;
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

/** A stored UI-language preference; "" means follow the device locale. */
export type AccountLanguage = 'en' | 'fr' | '';

/** The caller's own account, editable via `/me`. */
export interface Account {
  username: string;
  displayName: string;
  email: string;
  isAdmin: boolean;
  language: AccountLanguage;
}

/** A user's profile: identity, recent activity and public playlists. */
export interface ProfileResult {
  user: NonNullable<ProfileResponse['user']>;
  isSelf: boolean;
  isFriend: boolean;
  activity: ActivityEventDTO[];
  playlists: ProfilePlaylistDTO[];
}
