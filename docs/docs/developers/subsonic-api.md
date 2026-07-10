---
sidebar_position: 2
title: Subsonic API
---

# Subsonic API

Immerle implements two HTTP surfaces:

- **Immerle REST API** (`/api/v1/*`) — Immerle's own endpoints (accounts,
  social, providers, admin…). These are documented in the
  [API reference](pathname:///api/).
- **Subsonic / OpenSubsonic API** (`/rest/*`) — the standard music API that
  existing clients speak. It follows the upstream specification rather than a
  bespoke schema, so it lives here instead of in the OpenAPI reference.

## Calling convention

Every method is a path under `/rest/`, callable with or without the legacy
`.view` suffix:

```
GET /rest/ping
GET /rest/getAlbum?id=123
GET /rest/getAlbum.view?id=123
```

All methods are authenticated. Pass the standard Subsonic parameters on every
request:

| Param | Meaning |
| ----- | ------- |
| `u`   | Username |
| `t` + `s` | Auth token (`md5(password + salt)`) and salt — preferred |
| `p`   | Password (plaintext or `enc:` hex) — legacy alternative to `t`+`s` |
| `v`   | Protocol version the client targets |
| `c`   | Client application name |
| `f`   | Response format: `xml` (default), `json`, or `jsonp` |

An Immerle API token may also be supplied via the `Authorization` header
instead of `u`/`p`.

:::warning LDAP users must use password auth
If the account is backed by [LDAP](../configuration.md#ldap-authentication), token
auth (`t`+`s`) **cannot** work — the directory never exposes the plaintext the
server would need to recompute `md5(password + salt)`. Configure the client to
send the password (`p`) instead, and only over HTTPS.
:::

For full request/response schemas, see the upstream specs — Immerle aims to be
compatible with them:

- [OpenSubsonic API](https://opensubsonic.netlify.app/)
- [Subsonic API](https://www.subsonic.org/pages/api.jsp)

## Supported methods

### System

`ping` · `getLicense` · `getOpenSubsonicExtensions` · `getScanStatus` ·
`startScan`

### Browsing

`getMusicFolders` · `getIndexes` · `getMusicDirectory` · `getGenres` ·
`getArtists` · `getArtist` · `getAlbum` · `getSong` · `getArtistInfo` ·
`getArtistInfo2` · `getAlbumInfo` · `getAlbumInfo2` · `getSimilarSongs` ·
`getSimilarSongs2` · `getTopSongs` · `getVideos`

### Album & song lists

`getAlbumList` · `getAlbumList2` · `getRandomSongs` · `getSongsByGenre` ·
`getStarred` · `getStarred2` · `getNowPlaying`

### Searching

`search` · `search2` · `search3`

### Playlists

`getPlaylists` · `getPlaylist` · `createPlaylist` · `updatePlaylist` ·
`deletePlaylist`

### Media retrieval

`stream` · `download` · `getCoverArt` · `getLyrics` · `getLyricsBySongId`

### Media annotation

`star` · `unstar` · `setRating` · `scrobble`

### Bookmarks & play queue

`getBookmarks` · `getPlayQueue` · `savePlayQueue`

### Sharing

`getShares` · `createShare` · `updateShare` · `deleteShare`

### User management

`getUser` · `getUsers` · `createUser` · `updateUser` · `deleteUser` ·
`changePassword`

### Other

`getInternetRadioStations` · `getChatMessages`
