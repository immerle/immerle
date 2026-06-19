/**
 * Immerle-specific data model — everything that lives beyond the Subsonic
 * surface (capabilities discovery, on-demand catalog, federation, richer admin).
 */

/** Feature flags an instance advertises at `/capabilities`. */
export interface Capabilities {
  /** Server version string. */
  version: string;
  /** Immerle API revision, lets the client gate by contract version. */
  apiRevision: number;
  features: {
    /** Native Immerle session/JWT auth (vs. Subsonic-only). */
    immerleAuth: boolean;
    /** On-demand catalog: fetch tracks from third-party providers. */
    onDemandCatalog: boolean;
    /** Admin-managed dynamic content providers, configurable at runtime. */
    dynamicProviders: boolean;
    /** Admin runtime settings endpoint (`/admin/settings`). */
    runtimeSettings: boolean;
    /** Downloads eviction-sweep endpoints (`/admin/cleanup`). */
    cleanup: boolean;
    /** Editorial/reco playlists pushed from the federation hub. */
    federation: boolean;
    /** Real-time listening sessions. */
    jam: boolean;
    /** Real-time collaborative playlists. */
    collaborativePlaylists: boolean;
    /** Public playlists with opt-in subscriptions. */
    publicPlaylists: boolean;
    /** Import playlists from external platforms (e.g. Spotify). */
    playlistImport: boolean;
    /** Friends / activity feed. */
    social: boolean;
    /** Admin track management: list/edit metadata/edit cover/delete. */
    libraryAdmin: boolean;
    /** Richer admin surface (providers, jobs, transcode profiles). */
    adminExtended: boolean;
    /** Per-track / per-album offline download endpoints. */
    offlineDownloads: boolean;
    /** Internet radio stations (built-in + admin-managed custom). */
    internetRadio: boolean;
    /** Year-in-review ("Wrapped") stats endpoint (`/wrapped`). */
    wrapped: boolean;
  };
  /** Transcode formats the server can produce, for the quality picker. */
  transcoding?: { format: string; maxBitRate: number }[];
}

export type CapabilityFeature = keyof Capabilities['features'];

/** A Immerle session obtained from native auth (optional, instance-dependent). */
export interface ImmerleSession {
  token: string;
  refreshToken?: string;
  expiresAt?: number;
  userId: string;
  username: string;
  isAdmin: boolean;
}

// --- Admin: library --------------------------------------------------------

export interface LibraryStats {
  artistCount: number;
  albumCount: number;
  songCount: number;
  /** Bytes used by the media library. */
  totalSize: number;
  lastScan?: string;
}

/** Editable track metadata fields (all optional for partial updates). */
export interface TrackEdit {
  title?: string;
  genre?: string;
  year?: number;
  trackNo?: number;
  discNo?: number;
}

export interface ScanProgress {
  scanning: boolean;
  /** Items processed so far. */
  count: number;
  /** Total items if known (incremental scans may not report one). */
  total?: number;
  phase?: 'scanning' | 'cleaning' | 'analyzing' | 'idle';
  startedAt?: string;
}

// --- Admin: dynamic providers ----------------------------------------------

/**
 * An admin-managed dynamic provider: content-neutral, just a name + HTTP
 * endpoint + opaque JSON config. `enabled` is the stored intent; `active`
 * reflects whether it is registered in the live registry right now.
 */
export interface Provider {
  name: string;
  kind: string;
  endpoint: string;
  /** Opaque JSON config payload (headers, paths, quality…), as a string. */
  config: string;
  enabled: boolean;
  active: boolean;
  /** Built-in providers (from server config) can't be deleted or redefined. */
  builtin: boolean;
  /** False for built-ins; true for user-created dynamic providers. */
  deletable: boolean;
  /** Priority order (lower = higher priority); drives search fallback. */
  sortOrder: number;
  /** Live protocol version from the remote's /capabilities (HTTP providers). */
  version?: number;
}

/** A persisted warn/error event from a provider action (admin diagnostics). */
export interface ProviderLog {
  id: string;
  provider: string;
  level: 'warn' | 'error';
  /** 'search' | 'resolve' | 'download' */
  action: string;
  message: string;
  createdAt: string;
}

export type DownloadJobStatus =
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled';

export interface DownloadJob {
  id: string;
  providerId: string;
  query: string;
  title?: string;
  artist?: string;
  status: DownloadJobStatus;
  /** 0..1 */
  progress: number;
  error?: string;
  createdAt: string;
  /** Resulting Subsonic song id once imported. */
  resultSongId?: string;
}

// --- Admin: federation -----------------------------------------------------

export interface FederationState {
  enabled: boolean;
  hubUrl?: string;
  connection: 'connected' | 'connecting' | 'disconnected' | 'error';
  message?: string;
  /** Opt-in anonymized export of listening stats to the hub. */
  anonymizedExport: boolean;
  lastSync?: string;
}

// --- Admin: server / transcoding ------------------------------------------

export interface TranscodeProfile {
  id: string;
  name: string;
  targetFormat: string;
  maxBitRate: number;
  isDefault: boolean;
}

export interface ServerSettings {
  serverName?: string;
  defaultTranscodeProfileId?: string;
  scrobblingEnabled: boolean;
  /** Maximum bitrate streamed without explicit override. */
  maxStreamBitRate?: number;
}

// --- Internet radio --------------------------------------------------------

export interface RadioStation {
  id: string;
  name: string;
  streamUrl: string;
  homepageUrl: string;
  builtin: boolean;
  /** False for built-ins (editable but not removable). */
  deletable: boolean;
  /** True when the station has a logo (served by the cover endpoint). */
  hasCover?: boolean;
  /** Logo source URL (for prefilling the admin edit form). */
  coverUrl?: string;
  /** Country group code (e.g. "fr", "gb", "int"). */
  country?: string;
  /** True when the caller has favorited this station. */
  liked?: boolean;
}

// --- Wrapped (year-in-review) ----------------------------------------------

export interface WrappedTrack {
  id: string;
  title: string;
  artist: string;
  plays: number;
}

export interface WrappedCount {
  name: string;
  plays: number;
}

export interface Wrapped {
  year: number;
  totalPlays: number;
  totalSeconds: number;
  topTracks: WrappedTrack[] | null;
  topArtists: WrappedCount[] | null;
  topGenres: WrappedCount[] | null;
  /** Plays per calendar month, index 0 = January .. 11 = December. */
  byMonth: number[];
}

/** Thrown when a Immerle REST endpoint returns a non-2xx. */
export class ImmerleApiError extends Error {
  status: number;
  /** Stable server error code (e.g. `not_found`); i18n key under `errors.*`. */
  code?: string;
  /** Interpolation variables for the localized message (server-sent). */
  params?: Record<string, unknown>;
  constructor(status: number, message: string, code?: string, params?: Record<string, unknown>) {
    super(message);
    this.name = 'ImmerleApiError';
    this.status = status;
    this.code = code;
    this.params = params;
  }
}
