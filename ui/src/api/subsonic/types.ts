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
  /** Not yet downloaded (an on-demand provider result): plays via progressive
   * streaming, which can't serve byte ranges yet — seeking is unavailable
   * until the background download finishes and it's replayed. */
  remote?: boolean;
  /** A personal nostalgia note on this track within its playlist (e.g. a tier
   * list), like "listened to this in college". Only populated on a playlist's
   * tracks. */
  comment?: string;
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

/** A device this account is recently logged in on, offered as a playback-transfer target. */
export interface PlaybackTarget {
  id: string;
  name: string;
  lastUsedAt?: string;
}

/**
 * A spectator device's remote-control command (see ImmerleClient.sendPlayQueueCommand) —
 * an intent for the active device to apply itself, not a computed snapshot.
 */
export interface PlayQueueCommand {
  type: 'toggle' | 'next' | 'previous' | 'seekTo' | 'skipTo' | 'toggleShuffle' | 'cycleRepeat';
  /** Target position for a "seekTo" command. */
  positionMs?: number;
  /** Track to jump to for a "skipTo" command — resolved against the receiver's own queue. */
  trackId?: string;
  /** Disambiguates "skipTo" if trackId appears more than once in the queue — a hint only. */
  queueIndex?: number;
  /** The sender's view of the current active device id; ignored if the receiver isn't (or is no longer) that device. */
  forTarget?: string;
  /** The sending device's id. */
  issuedBy?: string;
}

/** The caller's saved cross-device play queue (see ImmerleClient.getPlayQueue). */
export interface PlayQueueSnapshot {
  songs: Song[];
  currentId?: string;
  positionMs: number;
  /** Whether currentId was playing (vs paused) as of this snapshot. */
  playing: boolean;
  /** The device id that should be the sole active player, or '' if unrestricted. */
  targetDeviceId: string;
  /** The device id that wrote this snapshot — tells "I wrote this" from "someone else did". */
  changedBy?: string;
  /** A spectator's not-yet-applied remote-control command, if any (see PlayQueueCommand). */
  pendingCommand?: PlayQueueCommand;
  /** Increases on every new command — lets a device tell a new one from one it already applied. */
  commandSeq: number;
  /** The saving device's shuffle/repeat mode — mirrored so another device that spectates or takes over this queue shows/resumes the same mode instead of its own possibly-stale local one. */
  shuffle: boolean;
  repeat: 'off' | 'track' | 'queue';
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
