import type {
  AdminUserView,
  AlbumView,
  ArtistView,
  FavoritesView,
  GenreView,
  NowPlayingView,
  PlaylistView,
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
  SearchResult3,
  Song,
  SubsonicUser,
} from '../subsonic/types';

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

export function toSearchResult(v: SearchView): SearchResult3 {
  return {
    artist: v.artists?.map(toArtist),
    album: v.albums?.map(toAlbum),
    song: v.songs?.map(toSong),
  };
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
    coverArts: v.coverArts,
  };
}

export function toPlaylistWithSongs(v: PlaylistView): PlaylistWithSongs {
  return { ...toPlaylist(v), entry: v.tracks?.map(toSong) };
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
