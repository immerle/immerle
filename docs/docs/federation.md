---
sidebar_position: 10
title: Federation
---

# Federation

Federation links your instance to an `immerle-hub` — an optional, opt-in
connection that syncs playlists between instances, resolves tracks by a
portable id even when you don't have the source instance's exact file, and
lets you export anonymized listening stats. Nothing is federated until you
link.

There's no `enabled` toggle: federation is active whenever your instance is
linked (has a hub-issued identity), and idle otherwise.

## Linking

1. Set your hub account id in the runtime settings:

   ```bash
   curl -X POST "http://host:4533/api/v1/admin/settings" -H 'Authorization: Bearer <admin>' \
     -H 'Content-Type: application/json' -d '{"federation":{"userId":"<your-hub-user-uuid>"}}'
   ```

2. Register — the server does the whole exchange with the hub itself and
   persists the identity it's issued:

   ```bash
   curl -X POST "http://host:4533/api/v1/admin/federation/register" -H 'Authorization: Bearer <admin>'
   ```

That returns the refreshed settings, now including a hub-assigned
`instanceId` (fixed) and `sqid` (an editable, unique handle — your instance's
public short name on the hub). The hub-issued private key is stored server
side and never returned in API responses.

```bash
GET    /api/v1/admin/federation               # current profile (refreshes name/sqid from the hub)
PATCH  /api/v1/admin/federation                # push a new name/sqid — {"name": "...", "sqid": "..."}
DELETE /api/v1/admin/federation                # unlink
```

## What syncs

Three independent things, each its own toggle in the runtime settings
(`syncPlaylists`, `resolveMissing`, `exportScrobbles` — all hot, no restart):

- **Playlist sync** — your **public, non-federated** playlists are pushed to
  the hub, deduplicated by content hash, covers uploaded content-addressed so
  identical art isn't re-uploaded across instances.
- **Portable-id resolution** — a federated playlist can reference a track by
  a portable id (e.g. MusicBrainz ID) your instance doesn't have a local file
  for. With `resolveMissing` on, Immerle first checks your library by that id
  and, if absent, searches the on-demand providers and downloads it —
  otherwise the track just isn't playable locally.
- **Scrobble export** — your listening counts are aggregated per track
  (identity and timestamps dropped) and sent as `hash(instanceId+trackId) →
  count`, so the hub can build aggregate stats without ever seeing which
  instance or user played what.

## Discovering other instances

```bash
GET  /api/v1/admin/federation/instances?query=       # search instances on the hub
GET  /api/v1/admin/federation/subscriptions          # instances you're subscribed to
POST /api/v1/admin/federation/subscriptions          # subscribe — {"instanceId": "..."}
DELETE /api/v1/admin/federation/subscriptions/{id}   # unsubscribe
```

Subscribing to another instance surfaces their federated playlists in your
own `getPlaylists`, read-only.

See the [API reference](pathname:///api/) for exact request/response shapes.
