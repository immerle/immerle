/**
 * Subset of the OpenSubsonic data model that the app consumes.
 *
 * Field names mirror the wire format (camelCase as emitted with `f=json`).
 * Only the fields the UI actually reads are typed; everything is optional
 * where the spec allows servers to omit it, so we stay tolerant of partial
 * implementations.
 */

export interface SubsonicError {
  code: number;
  message: string;
}

/** The envelope every endpoint wraps its payload in. */
export interface SubsonicResponse<T = Record<string, unknown>> {
  'subsonic-response': T & {
    status: 'ok' | 'failed';
    version: string;
    type?: string;
    serverVersion?: string;
    openSubsonic?: boolean;
    error?: SubsonicError;
  };
}

export interface Artist {
  id: string;
  name: string;
  coverArt?: string;
  artistImageUrl?: string;
  albumCount?: number;
  starred?: string;
}

export interface ArtistWithAlbums extends Artist {
  album?: Album[];
}

export interface Album {
  id: string;
  name: string;
  artist?: string;
  artistId?: string;
  coverArt?: string;
  songCount?: number;
  duration?: number;
  playCount?: number;
  created?: string;
  starred?: string;
  year?: number;
  genre?: string;
}

export interface AlbumWithSongs extends Album {
  song?: Song[];
}

export interface Song {
  id: string;
  parent?: string;
  title: string;
  album?: string;
  artist?: string;
  track?: number;
  year?: number;
  genre?: string;
  coverArt?: string;
  /** Pre-built cover URL for non-Subsonic art (e.g. radio logos), bypassing the
   * coverArt-id resolution. */
  coverUrl?: string;
  size?: number;
  contentType?: string;
  suffix?: string;
  transcodedContentType?: string;
  transcodedSuffix?: string;
  duration?: number;
  bitRate?: number;
  path?: string;
  albumId?: string;
  artistId?: string;
  type?: string;
  isDir?: boolean;
  isVideo?: boolean;
  starred?: string;
  discNumber?: number;
  /** A federated-playlist entry not yet matched to a playable track (name-only;
   * resolve via playlists.resolveTrack before playing). */
  unresolved?: boolean;
}

export interface Genre {
  value: string;
  songCount: number;
  albumCount: number;
}

/** An active playback session from `getNowPlaying` (song fields + who/when/where). */
export interface NowPlayingEntry extends Song {
  username?: string;
  minutesAgo?: number;
  playerId?: number;
  playerName?: string;
}

export interface Playlist {
  id: string;
  name: string;
  comment?: string;
  owner?: string;
  public?: boolean;
  songCount?: number;
  duration?: number;
  created?: string;
  changed?: string;
  coverArt?: string;
  /** Cover-art ids of the first up to 4 tracks, for a mosaic thumbnail. */
  coverArts?: string[];
  /** Read-only playlist synced from the hub: `owner` is only an internal
   * attribution, never real ownership — never show edit/delete/cover
   * controls for it, only subscribe/unsubscribe. */
  federated?: boolean;
  /** Whether the caller has favorited this playlist. Only populated by
   * getPlaylist (the single-playlist resource) — absent from playlist lists. */
  subscribed?: boolean;
}

export interface PlaylistWithSongs extends Playlist {
  entry?: Song[];
}

export interface SearchResult3 {
  artist?: Artist[];
  album?: Album[];
  song?: Song[];
}

export interface ArtistsIndex {
  index?: { name: string; artist: Artist[] }[];
  ignoredArticles?: string;
}

export interface ScanStatus {
  scanning: boolean;
  count?: number;
}

/** OpenSubsonic `getUser` / `getUsers` shape. */
export interface SubsonicUser {
  username: string;
  /** Free-text name shown in the UI; falls back to `username` when empty. */
  displayName?: string;
  email?: string;
  scrobblingEnabled?: boolean;
  adminRole?: boolean;
  settingsRole?: boolean;
  downloadRole?: boolean;
  uploadRole?: boolean;
  playlistRole?: boolean;
  coverArtRole?: boolean;
  commentRole?: boolean;
  podcastRole?: boolean;
  streamRole?: boolean;
  jukeboxRole?: boolean;
  shareRole?: boolean;
  folder?: number[];
}

/** Thrown when the server returns `status: "failed"`. */
export class SubsonicApiError extends Error {
  code: number;
  constructor(error: SubsonicError) {
    super(error.message || `Subsonic error ${error.code}`);
    this.name = 'SubsonicApiError';
    this.code = error.code;
  }
}
