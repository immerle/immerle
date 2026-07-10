---
sidebar_position: 8
title: Social features
---

# Social features

Beyond streaming your own library, Immerle has a small social layer: friends,
an activity feed, playlists you can share or collaborate on, and synchronized
listening sessions. All of it is opt-in per action — nothing is public by
default.

These are native Immerle extensions (`/api/v1/*`), not part of the Subsonic
API — see the [API reference](pathname:///api/) for exact request/response
shapes. Discover what a server supports with the unauthenticated
`GET /api/v1/capabilities`.

## Friends & activity

A friend relationship is a simple request/accept pair:

```bash
curl -X POST "http://host:4533/api/v1/friends/requests" -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' -d '{"username":"alex"}'

# alex accepts
curl -X POST "http://host:4533/api/v1/friends/requests/kilian/accept" -H 'Authorization: Bearer <alex-token>'

curl "http://host:4533/api/v1/friends" -H 'Authorization: Bearer <token>'          # accepted friends
curl "http://host:4533/api/v1/friends/requests" -H 'Authorization: Bearer <token>' # pending, incoming
```

Plays, likes, playlist creations, etc. feed an **activity feed**, each event
tagged with a privacy level:

| Privacy | Visible to |
| ------- | ---------- |
| `public` | anyone |
| `friends` | accepted friends only |
| `private` | the author only |

`GET /api/v1/profile?username=<name>` returns someone's identity, the
activity they've chosen to expose to *you* specifically, their public
playlists, and `isFriend`/`isSelf` flags. Omit `username` to fetch your own —
`/account` is the equivalent for your own **editable** account (email,
display name), which a public profile never exposes.

## Collaborative & public playlists

An owner opts a playlist into two independent things:

- **Public** (`updatePlaylist?public=true`, Subsonic) — visible to anyone via
  `GET /playlists/public`, not auto-added to other users' libraries.
- **Collaborative** — specific users can edit it.

```bash
# discover public playlists
curl "http://host:4533/api/v1/playlists/public" -H 'Authorization: Bearer <token>'

# subscribe: it now behaves like a normal (read-only) playlist in your library
curl -X PUT "http://host:4533/api/v1/playlists/<id>/subscription" -H 'Authorization: Bearer <token>'
curl -X DELETE "http://host:4533/api/v1/playlists/<id>/subscription" -H 'Authorization: Bearer <token>'

# owner grants edit rights to another user
curl -X POST "http://host:4533/api/v1/playlists/<id>/collaborators" -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' -d '{"username":"alex"}'
```

A subscriber can't modify the playlist — in a Subsonic client, "deleting" a
subscribed playlist just unsubscribes; the owner's copy is untouched.
`getPlaylists` returns your own, collaborative, subscribed and federated
playlists together.

## Share links

A share is a **public, unauthenticated** link to a single track, album or
playlist — for sending to someone who doesn't have an account:

```bash
curl -X POST "http://host:4533/api/v1/shares" -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{"itemId":"<id>","description":"check this out","expiresAt":1780000000000}'
# → resolves to {baseURL}/share/{secret}
```

`expiresAt` (epoch milliseconds) is optional — omit it for a link that never
expires. `PATCH`/`DELETE /shares/{id}` update or revoke it; only the owner can.

## Jam sessions

A Jam is a host-controlled, synchronized listening session streamed to
participants over Server-Sent Events — everyone hears the same track at the
same position in real time.

```bash
# host creates a session with a starting queue
curl -X POST "http://host:4533/api/v1/jam" -H 'Authorization: Bearer <host-token>' \
  -H 'Content-Type: application/json' -d '{"name":"Friday night","trackIds":["t1","t2","t3"]}'
# → { "id": "<jamId>", ... }

# anyone joins/leaves freely
curl -X POST "http://host:4533/api/v1/jam/<id>/participants" -H 'Authorization: Bearer <token>'
curl -X DELETE "http://host:4533/api/v1/jam/<id>/participants/me" -H 'Authorization: Bearer <token>'

# subscribe to live state (SSE) — emits a `state` event immediately and on every change
curl -N "http://host:4533/api/v1/jam/<id>/events" -H 'Authorization: Bearer <token>'

# host-only: control playback
curl -X PATCH "http://host:4533/api/v1/jam/<id>" -H 'Authorization: Bearer <host-token>' \
  -H 'Content-Type: application/json' -d '{"currentTrackId":"t2","position":45.2,"state":"playing"}'

# host-only: end the session
curl -X DELETE "http://host:4533/api/v1/jam/<id>" -H 'Authorization: Bearer <host-token>'
```

Only the host can change playback state or end the session (a non-host
attempt gets `403`); joining and leaving are open to any authenticated user.
The event stream sends a heartbeat every 20s to keep long-lived connections
alive through proxies.
