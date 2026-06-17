import { md5 } from './md5';
import {
  Album,
  AlbumWithSongs,
  Artist,
  ArtistsIndex,
  ArtistWithAlbums,
  Genre,
  NowPlayingEntry,
  Playlist,
  PlaylistWithSongs,
  ScanStatus,
  SearchResult3,
  Song,
  SubsonicApiError,
  SubsonicResponse,
  SubsonicUser,
} from './types';

export const SUBSONIC_API_VERSION = '1.16.1';
const CLIENT_NAME = 'immerle';

/** Credentials needed to talk to a Subsonic server. The raw password is never kept. */
export interface SubsonicCredentials {
  serverUrl: string;
  username: string;
  /** Random per-session salt. */
  salt: string;
  /** md5(password + salt). */
  token: string;
}

/** Derive token-auth credentials from a password. Run once at login. */
export function deriveCredentials(
  serverUrl: string,
  username: string,
  password: string,
  salt: string,
): SubsonicCredentials {
  return {
    serverUrl: normalizeServerUrl(serverUrl),
    username,
    salt,
    token: md5(password + salt),
  };
}

/**
 * Normalize a user-typed server URL. When no scheme is given we default to
 * `https://` for public hosts, but `http://` for loopback / LAN addresses —
 * self-hosted Subsonic servers there usually speak plain HTTP, and forcing
 * https would make every request fail the TLS/ALPN handshake.
 */
export function normalizeServerUrl(url: string): string {
  let u = url.trim();
  if (!/^https?:\/\//i.test(u)) {
    const host = u.split('/')[0].split(':')[0].toLowerCase();
    const isLocal =
      host === 'localhost' ||
      host === '127.0.0.1' ||
      host === '::1' ||
      host === '[::1]' ||
      host.endsWith('.local') ||
      /^10\./.test(host) ||
      /^192\.168\./.test(host) ||
      /^172\.(1[6-9]|2\d|3[01])\./.test(host);
    u = `${isLocal ? 'http' : 'https'}://${u}`;
  }
  return u.replace(/\/+$/, '');
}

export type QueryParams = Record<
  string,
  string | number | boolean | undefined | (string | number)[]
>;

/**
 * Thin, typed wrapper over the Subsonic REST API.
 *
 * Every request is authenticated with the salted-token scheme
 * (`u`, `t`, `s`, `v`, `c`, `f=json`). The client exposes both raw helpers
 * (`get`, `buildUrl`) and typed endpoint methods. Streaming/cover-art URLs are
 * built synchronously so they can be handed straight to the audio engine or
 * `<Image>`.
 */
export class SubsonicClient {
  constructor(private readonly creds: SubsonicCredentials) {}

  get serverUrl(): string {
    return this.creds.serverUrl;
  }

  get username(): string {
    return this.creds.username;
  }

  /** Salted-token auth query params shared by every authenticated URL. */
  private authSearch(params: QueryParams = {}): URLSearchParams {
    const search = new URLSearchParams({
      u: this.creds.username,
      t: this.creds.token,
      s: this.creds.salt,
      v: SUBSONIC_API_VERSION,
      c: CLIENT_NAME,
      f: 'json',
    });
    for (const [key, value] of Object.entries(params)) {
      if (value === undefined) continue;
      if (Array.isArray(value)) {
        for (const v of value) search.append(key, String(v));
      } else {
        search.append(key, String(value));
      }
    }
    return search;
  }

  /** Build a fully-authenticated URL for a Subsonic `/rest` endpoint. */
  buildUrl(endpoint: string, params: QueryParams = {}): string {
    return `${this.creds.serverUrl}/rest/${endpoint}?${this.authSearch(params)}`;
  }

  /**
   * Salted-token auth params, for layering onto the Immerle extension API
   * (which accepts the same `u`+`t`+`s`+`v`+`c` scheme as Subsonic).
   */
  tokenParams(): { u: string; t: string; s: string; v: string; c: string } {
    return {
      u: this.creds.username,
      t: this.creds.token,
      s: this.creds.salt,
      v: SUBSONIC_API_VERSION,
      c: CLIENT_NAME,
    };
  }

  /**
   * Build an authenticated URL for a non-`/rest` path (the Immerle extension
   * API, mounted at the server root with no prefix). Carries the same
   * salted-token auth params.
   */
  authedUrl(path: string, params: QueryParams = {}): string {
    const clean = path.replace(/^\/+/, '');
    return `${this.creds.serverUrl}/${clean}?${this.authSearch(params)}`;
  }

  /** Perform a JSON request and unwrap the `subsonic-response` envelope. */
  async get<T extends Record<string, unknown>>(
    endpoint: string,
    params: QueryParams = {},
    signal?: AbortSignal,
  ): Promise<T> {
    const res = await fetch(this.buildUrl(endpoint, params), { signal });
    if (!res.ok) {
      throw new SubsonicApiError({ code: res.status, message: `HTTP ${res.status}` });
    }
    const json = (await res.json()) as SubsonicResponse<T>;
    const body = json['subsonic-response'];
    if (!body || body.status === 'failed') {
      throw new SubsonicApiError(
        body?.error ?? { code: -1, message: 'Malformed Subsonic response' },
      );
    }
    return body as T;
  }

  // --- Connectivity --------------------------------------------------------

  async ping(): Promise<{ ok: boolean; openSubsonic: boolean; serverVersion?: string }> {
    const r = await this.get('ping.view');
    return {
      ok: true,
      openSubsonic: Boolean(r.openSubsonic),
      serverVersion: r.serverVersion as string | undefined,
    };
  }

  // --- Browsing ------------------------------------------------------------

  async getArtists(): Promise<Artist[]> {
    const r = await this.get<{ artists: ArtistsIndex }>('getArtists.view');
    return (r.artists?.index ?? []).flatMap((i) => i.artist ?? []);
  }

  async getArtist(id: string): Promise<ArtistWithAlbums> {
    const r = await this.get<{ artist: ArtistWithAlbums }>('getArtist.view', { id });
    return r.artist;
  }

  async getAlbum(id: string): Promise<AlbumWithSongs> {
    const r = await this.get<{ album: AlbumWithSongs }>('getAlbum.view', { id });
    return r.album;
  }

  async getSong(id: string): Promise<Song> {
    const r = await this.get<{ song: Song }>('getSong.view', { id });
    return r.song;
  }

  /**
   * `type`: one of newest, recent, frequent, random, alphabeticalByName,
   * alphabeticalByArtist, starred, byYear, byGenre.
   */
  async getAlbumList(
    type: string,
    opts: { size?: number; offset?: number; genre?: string } = {},
  ): Promise<Album[]> {
    const r = await this.get<{ albumList2: { album?: Album[] } }>('getAlbumList2.view', {
      type,
      size: opts.size ?? 50,
      offset: opts.offset ?? 0,
      genre: opts.genre,
    });
    return r.albumList2?.album ?? [];
  }

  async getGenres(): Promise<Genre[]> {
    const r = await this.get<{ genres: { genre?: Genre[] } }>('getGenres.view');
    return r.genres?.genre ?? [];
  }

  async getSongsByGenre(genre: string, count = 100, offset = 0): Promise<Song[]> {
    const r = await this.get<{ songsByGenre: { song?: Song[] } }>('getSongsByGenre.view', {
      genre,
      count,
      offset,
    });
    return r.songsByGenre?.song ?? [];
  }

  async getRandomSongs(size = 50, genre?: string): Promise<Song[]> {
    const r = await this.get<{ randomSongs: { song?: Song[] } }>('getRandomSongs.view', {
      size,
      genre,
    });
    return r.randomSongs?.song ?? [];
  }

  async getStarred(): Promise<SearchResult3> {
    const r = await this.get<{ starred2: SearchResult3 }>('getStarred2.view');
    return r.starred2 ?? {};
  }

  /** Active playback sessions across the server ("connected devices"). */
  async getNowPlaying(): Promise<NowPlayingEntry[]> {
    const r = await this.get<{ nowPlaying: { entry?: NowPlayingEntry[] } }>('getNowPlaying.view');
    return r.nowPlaying?.entry ?? [];
  }

  // --- Search --------------------------------------------------------------

  async search(
    query: string,
    opts: {
      artistCount?: number;
      albumCount?: number;
      songCount?: number;
      artistOffset?: number;
      albumOffset?: number;
      songOffset?: number;
    } = {},
  ): Promise<SearchResult3> {
    const r = await this.get<{ searchResult3: SearchResult3 }>('search3.view', {
      query,
      artistCount: opts.artistCount ?? 20,
      albumCount: opts.albumCount ?? 20,
      songCount: opts.songCount ?? 30,
      artistOffset: opts.artistOffset,
      albumOffset: opts.albumOffset,
      songOffset: opts.songOffset,
    });
    return r.searchResult3 ?? {};
  }

  // --- Playlists -----------------------------------------------------------

  async getPlaylists(): Promise<Playlist[]> {
    const r = await this.get<{ playlists: { playlist?: Playlist[] } }>('getPlaylists.view');
    return r.playlists?.playlist ?? [];
  }

  async getPlaylist(id: string): Promise<PlaylistWithSongs> {
    const r = await this.get<{ playlist: PlaylistWithSongs }>('getPlaylist.view', { id });
    return r.playlist;
  }

  async createPlaylist(name: string, songIds: string[] = []): Promise<void> {
    await this.get('createPlaylist.view', { name, songId: songIds });
  }

  /**
   * Update a playlist. `songIndexToRemove` removes entries by position;
   * `songIdToAdd` appends songs. Reordering is done by the higher-level
   * Immerle client which replaces the whole entry list.
   */
  async updatePlaylist(
    playlistId: string,
    opts: {
      name?: string;
      comment?: string;
      public?: boolean;
      songIdToAdd?: string[];
      songIndexToRemove?: number[];
    },
  ): Promise<void> {
    await this.get('updatePlaylist.view', {
      playlistId,
      name: opts.name,
      comment: opts.comment,
      public: opts.public,
      songIdToAdd: opts.songIdToAdd,
      songIndexToRemove: opts.songIndexToRemove,
    });
  }

  async deletePlaylist(id: string): Promise<void> {
    await this.get('deletePlaylist.view', { id });
  }

  // --- Annotation / scrobbling --------------------------------------------

  async star(opts: { id?: string; albumId?: string; artistId?: string }): Promise<void> {
    await this.get('star.view', opts);
  }

  async unstar(opts: { id?: string; albumId?: string; artistId?: string }): Promise<void> {
    await this.get('unstar.view', opts);
  }

  async setRating(id: string, rating: number): Promise<void> {
    await this.get('setRating.view', { id, rating });
  }

  /** `submission=false` marks a "now playing" event; `true` is a real scrobble. */
  async scrobble(id: string, submission: boolean, time?: number): Promise<void> {
    await this.get('scrobble.view', { id, submission, time });
  }

  // --- Play queue sync -----------------------------------------------------

  async savePlayQueue(songIds: string[], current?: string, positionMs?: number): Promise<void> {
    await this.get('savePlayQueue.view', { id: songIds, current, position: positionMs });
  }

  async getPlayQueue(): Promise<{ current?: string; position?: number; entry?: Song[] }> {
    const r = await this.get<{ playQueue?: { current?: string; position?: number; entry?: Song[] } }>(
      'getPlayQueue.view',
    );
    return r.playQueue ?? {};
  }

  // --- Users (admin; standard Subsonic) -----------------------------------

  async getUsers(): Promise<SubsonicUser[]> {
    const r = await this.get<{ users: { user?: SubsonicUser[] } }>('getUsers.view');
    return r.users?.user ?? [];
  }

  async getUser(username: string): Promise<SubsonicUser> {
    const r = await this.get<{ user: SubsonicUser }>('getUser.view', { username });
    return r.user;
  }

  async createUser(params: {
    username: string;
    password: string;
    displayName?: string;
    email?: string;
    adminRole?: boolean;
    settingsRole?: boolean;
    streamRole?: boolean;
    downloadRole?: boolean;
    playlistRole?: boolean;
  }): Promise<void> {
    await this.get('createUser.view', { ...params });
  }

  async updateUser(params: {
    username: string;
    displayName?: string;
    email?: string;
    adminRole?: boolean;
    settingsRole?: boolean;
    streamRole?: boolean;
    downloadRole?: boolean;
    playlistRole?: boolean;
  }): Promise<void> {
    await this.get('updateUser.view', { ...params });
  }

  async deleteUser(username: string): Promise<void> {
    await this.get('deleteUser.view', { username });
  }

  async changePassword(username: string, password: string): Promise<void> {
    await this.get('changePassword.view', { username, password });
  }

  // --- Library scanning (standard Subsonic) -------------------------------

  async startScan(): Promise<ScanStatus> {
    const r = await this.get<{ scanStatus: ScanStatus }>('startScan.view');
    return r.scanStatus;
  }

  async getScanStatus(): Promise<ScanStatus> {
    const r = await this.get<{ scanStatus: ScanStatus }>('getScanStatus.view');
    return r.scanStatus;
  }

  // --- Media URLs (synchronous) -------------------------------------------

  /** Streaming URL. `maxBitRate`/`format` drive server-side transcoding. */
  streamUrl(
    id: string,
    opts: { maxBitRate?: number; format?: string; estimateContentLength?: boolean } = {},
  ): string {
    return this.buildUrl('stream.view', {
      id,
      maxBitRate: opts.maxBitRate,
      format: opts.format,
      estimateContentLength: opts.estimateContentLength,
    });
  }

  /** Original-file download URL (no transcoding). */
  downloadUrl(id: string): string {
    return this.buildUrl('download.view', { id });
  }

  /** Cover-art URL. `size` requests a square thumbnail. */
  coverArtUrl(coverArtId: string | undefined, size?: number): string | undefined {
    if (!coverArtId) return undefined;
    return this.buildUrl('getCoverArt.view', { id: coverArtId, size });
  }
}
