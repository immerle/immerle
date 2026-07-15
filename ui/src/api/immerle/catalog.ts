import type {
  AdminUserView,
  AlbumView,
  ArtistView,
  FavoritesView,
  GenreView,
  HallOfFameView,
  NowPlayingView,
  PlaylistView,
  PlayQueueView,
  SearchView,
  SongView,
} from '../immerleApi';
import type {
  Album,
  AlbumWithSongs,
  Artist,
  ArtistWithAlbums,
  Genre,
  NowPlayingEntry,
  Playlist,
  PlaylistWithSongs,
  PlayQueueCommand,
  PlayQueueSnapshot,
  Song,
  SubsonicUser,
} from '../subsonic/types';
import type { HallOfFame } from './types';

/**
 * Map the native catalog DTOs (generated from the OpenAPI spec, where every
 * field is optional) into the app's clean domain types, the same way the rest
 * of the Immerle client normalizes generated DTOs (see `toProvider`). This keeps
 * the screens and player working against required-field domain types while the
 * data is sourced entirely from the native `/api/v1` catalog endpoints.
 */

export function toSong(v: SongView): Song {
  return {
    id: v.id ?? '',
    title: v.title ?? '',
    album: v.album,
    artist: v.artist,
    albumId: v.albumId,
    artistId: v.artistId,
    coverArt: v.coverArt,
    duration: v.duration,
    track: v.track,
    year: v.year,
    genre: v.genre,
    suffix: v.suffix,
    contentType: v.contentType,
    size: v.size,
    starred: v.starred,
    unresolved: v.unresolved,
    remote: v.remote,
    comment: v.comment,
  };
}

/**
 * Shared between ImmerleClient.getPlayQueue's typed response and the raw SSE
 * "state" event payload (see connectPlayQueueLive in ui/src/audio/store.ts) —
 * both carry the same wire shape, just one arrives pre-parsed by openapi-fetch
 * and the other via JSON.parse on a raw event.
 */
export function toPlayQueueSnapshot(v: PlayQueueView): PlayQueueSnapshot {
  return {
    songs: (v.entries ?? []).map(toSong),
    currentId: v.current || undefined,
    positionMs: v.position ?? 0,
    playing: !!v.playing,
    changedBy: v.changedBy || undefined,
    targetDeviceId: v.targetDeviceId ?? '',
    pendingCommand: v.pendingCommand
      ? {
          type: v.pendingCommand.type as PlayQueueCommand['type'],
          positionMs: v.pendingCommand.positionMs,
          trackId: v.pendingCommand.trackId,
          queueIndex: v.pendingCommand.queueIndex,
          forTarget: v.pendingCommand.forTarget,
          issuedBy: v.pendingCommand.issuedBy,
        }
      : undefined,
    commandSeq: v.commandSeq ?? 0,
    shuffle: !!v.shuffle,
    repeat: (v.repeat as PlayQueueSnapshot['repeat']) || 'off',
  };
}

export function toAlbum(v: AlbumView): Album {
  return {
    id: v.id ?? '',
    name: v.name ?? '',
    artist: v.artist,
    artistId: v.artistId,
    coverArt: v.coverArt,
    songCount: v.songCount,
    duration: v.duration,
    year: v.year,
    genre: v.genre,
    starred: v.starred,
  };
}

export function toAlbumWithSongs(v: AlbumView): AlbumWithSongs {
  return { ...toAlbum(v), song: v.tracks?.map(toSong) };
}

export function toArtist(v: ArtistView): Artist {
  return {
    id: v.id ?? '',
    name: v.name ?? '',
    coverArt: v.coverArt,
    albumCount: v.albumCount,
    starred: v.starred,
  };
}

export function toArtistWithAlbums(v: ArtistView): ArtistWithAlbums {
  return { ...toArtist(v), album: v.albums?.map(toAlbum) };
}

export function toGenre(v: GenreView): Genre {
  return { value: v.name ?? '', songCount: v.songCount ?? 0, albumCount: v.albumCount ?? 0 };
}

/** The Subsonic-style `getStarred` shape, mapped from the native favorites DTO. */
export interface Starred {
  artist?: Artist[];
  album?: Album[];
  song?: Song[];
}

export function toStarred(v: FavoritesView): Starred {
  return {
    artist: v.artists?.map(toArtist),
    album: v.albums?.map(toAlbum),
    song: v.songs?.map(toSong),
  };
}

/**
 * One global-search match: exactly one of artist/album/song/playlist is set,
 * per `type`. Unlike the Subsonic-style SearchResult3 (grouped by type),
 * this is a single list already ranked by relevance to the query.
 */
export type SearchHit =
  | { type: 'artist'; artist: Artist }
  | { type: 'album'; album: Album }
  | { type: 'song'; song: Song }
  | { type: 'playlist'; playlist: Playlist };

export function toSearchResult(v: SearchView): SearchHit[] {
  const hits: SearchHit[] = [];
  for (const r of v.results ?? []) {
    if (r.type === 'artist' && r.artist) hits.push({ type: 'artist', artist: toArtist(r.artist) });
    else if (r.type === 'album' && r.album) hits.push({ type: 'album', album: toAlbum(r.album) });
    else if (r.type === 'song' && r.song) hits.push({ type: 'song', song: toSong(r.song) });
    else if (r.type === 'playlist' && r.playlist) hits.push({ type: 'playlist', playlist: toPlaylist(r.playlist) });
  }
  return hits;
}

export function toPlaylist(v: PlaylistView): Playlist {
  return {
    id: v.id ?? '',
    name: v.name ?? '',
    comment: v.comment,
    owner: v.owner,
    public: v.public,
    songCount: v.songCount,
    duration: v.duration,
    created: v.createdAt,
    changed: v.changedAt,
    coverArt: v.coverArt,
    coverArts: v.coverArts,
    federated: v.federated,
    subscribed: v.subscribed,
  };
}

export function toPlaylistWithSongs(v: PlaylistView): PlaylistWithSongs {
  return { ...toPlaylist(v), entry: v.tracks?.map(toSong) };
}

export function toHallOfFame(v: HallOfFameView): HallOfFame {
  return { entries: (v.tracks ?? []).map(toSong) };
}

export function toNowPlaying(v: NowPlayingView): NowPlayingEntry {
  return {
    ...(v.song ? toSong(v.song) : { id: '', title: '' }),
    username: v.username,
    minutesAgo: v.minutesAgo,
  };
}

export function toSubsonicUser(v: AdminUserView): SubsonicUser {
  return {
    username: v.username ?? '',
    displayName: v.displayName,
    email: v.email,
    adminRole: v.admin,
    scrobblingEnabled: v.scrobblingEnabled,
  };
}
