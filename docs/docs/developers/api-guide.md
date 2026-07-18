---
sidebar_position: 4
title: Native API walkthrough
---

# Native API walkthrough

Worked `curl` examples for the native Immerle extensions (`/api/v1/*`). Each
section here pairs with an operator-facing page that explains *why* you'd use
the feature: [On-demand catalog](../on-demand-providers.md),
[Social features](../social.md), [Playlist import](../playlist-import.md) and
[Federation](../federation.md). For exact request/response schemas, see the
[API reference](pathname:///api/) (generated from the same handlers as these
examples).

All examples below use `Authorization: Bearer <token>` — either a device JWT
(from `POST /auth/sessions`) or a personal API token (`gsk_…`). Admin-only
endpoints need a token belonging to an admin account.

## Sessions & tokens

```bash
# log in → a device JWT (store it; send as Authorization: Bearer)
curl -X POST "http://host:4533/api/v1/auth/sessions" -H 'Content-Type: application/json' \
  -d '{"username":"me","password":"pw","device":"MacBook"}'
# → { "token": "eyJ…", "device": { "id": "<jti>", ... } }

# see / revoke your device sessions
curl -H "Authorization: Bearer eyJ…" "http://host:4533/api/v1/devices"
curl -X DELETE -H "Authorization: Bearer eyJ…" "http://host:4533/api/v1/devices/<jti>"

# mint a personal API token (secret returned ONCE — store it now)
curl -X POST "http://host:4533/api/v1/tokens" -H "Authorization: Bearer eyJ…" \
  -H 'Content-Type: application/json' -d '{"name":"my-cli"}'
# → { "token": "gsk_…", "id": "…", "name": "my-cli", "prefix": "gsk_…" }

# use it — as a Bearer header or ?apiKey, on BOTH the Subsonic and native APIs
curl -H "Authorization: Bearer gsk_…" "http://host:4533/rest/getArtists?c=app&f=json"
curl "http://host:4533/rest/getArtists?c=app&f=json&apiKey=gsk_…"

curl -H "Authorization: Bearer gsk_…" "http://host:4533/api/v1/tokens"
curl -X DELETE -H "Authorization: Bearer gsk_…" "http://host:4533/api/v1/tokens/<tokenId>"
```

## Runtime settings (admin)

```bash
curl "http://host:4533/api/v1/admin/settings" -H 'Authorization: Bearer <admin>'

# hot: tune provider behaviour (applies now)
curl -X POST "http://host:4533/api/v1/admin/settings" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' \
  -d '{"providers":{"autoDownloadOnPlay":true,"searchTimeoutSeconds":8}}'

# restart-required: toggling the scan watcher → response has restartRequired:true
curl -X POST "http://host:4533/api/v1/admin/settings" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' -d '{"scan":{"watch":false}}'
```

`POST` is a **partial** update — only fields present in the body change. When
a change needs a restart, the response sets `restartRequired: true` and lists
the affected fields in `pendingRestart`.

## On-demand providers

```bash
# list all providers with enabled/active/builtin/deletable/sortOrder/version
curl "http://host:4533/api/v1/admin/providers" -H 'Authorization: Bearer <admin>'

# fill in a built-in's credentials and enable it
curl -X POST "http://host:4533/api/v1/admin/providers" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' \
  -d '{"name":"jamendo","config":"{\"params\":{\"client_id\":\"<JAMENDO_TOKEN>\",\"audioformat\":\"mp32\"}}"}'
curl -X PUT "http://host:4533/api/v1/admin/providers/jamendo/enabled" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' -d '{"enabled":true}'

# add a dynamic HTTP provider by URL — name + config skeleton come from its /capabilities
curl -X POST "http://host:4533/api/v1/admin/providers" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' -d '{"endpoint":"https://my-service.internal"}'

curl -X PUT "http://host:4533/api/v1/admin/providers/order" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' -d '{"order":["free-music-archive","internet-archive","jamendo"]}'
curl -X DELETE "http://host:4533/api/v1/admin/providers/my-service" -H 'Authorization: Bearer <admin>'

# unused-download cleanup sweep
curl "http://host:4533/api/v1/admin/cleanup" -H 'Authorization: Bearer <admin>'
curl -X POST "http://host:4533/api/v1/admin/cleanup" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' -d '{"enabled":true}'
curl -X POST "http://host:4533/api/v1/admin/cleanup/run" -H 'Authorization: Bearer <admin>'
```

See [Building a custom content provider](./custom-provider.md) for the exact
`/capabilities`/`/search`/`/resolve`/`/download` contract a dynamic provider
must implement.

## Activity & profiles

```bash
# someone's profile (activity, public playlists, isSelf) — omit username for your own
curl "http://host:4533/api/v1/profile?username=alex" -H 'Authorization: Bearer <token>'

# your own editable account (email, display name) — never exposed on public profiles
curl "http://host:4533/api/v1/account" -H 'Authorization: Bearer <token>'
curl -X POST "http://host:4533/api/v1/account" -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' -d '{"displayName":"Kilian"}'
```

## Playlists: public, collaborative, shared

```bash
curl "http://host:4533/api/v1/playlists/public" -H 'Authorization: Bearer <token>'
curl -X PUT "http://host:4533/api/v1/playlists/<id>/subscription" -H 'Authorization: Bearer <token>'
curl -X DELETE "http://host:4533/api/v1/playlists/<id>/subscription" -H 'Authorization: Bearer <token>'
curl -X POST "http://host:4533/api/v1/playlists/<id>/collaborators" -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' -d '{"username":"alex"}'

# public, unauthenticated share link (expiresAt is epoch millis, optional)
curl -X POST "http://host:4533/api/v1/shares" -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{"itemId":"<id>","description":"check this out","expiresAt":1780000000000}'
curl -X PATCH "http://host:4533/api/v1/shares/<id>" -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' -d '{"description":"updated"}'
curl -X DELETE "http://host:4533/api/v1/shares/<id>" -H 'Authorization: Bearer <token>'
```

## Jam sessions

```bash
# host creates a session with a starting queue
curl -X POST "http://host:4533/api/v1/jam" -H 'Authorization: Bearer <host-token>' \
  -H 'Content-Type: application/json' -d '{"name":"Friday night","trackIds":["t1","t2","t3"]}'

# anyone joins/leaves freely
curl -X POST "http://host:4533/api/v1/jam/<id>/participants" -H 'Authorization: Bearer <token>'
curl -X DELETE "http://host:4533/api/v1/jam/<id>/participants/me" -H 'Authorization: Bearer <token>'

# live state over SSE — a `state` event immediately, then on every change, heartbeat every 20s
curl -N "http://host:4533/api/v1/jam/<id>/events" -H 'Authorization: Bearer <token>'

# host-only: control playback / end the session
curl -X PATCH "http://host:4533/api/v1/jam/<id>" -H 'Authorization: Bearer <host-token>' \
  -H 'Content-Type: application/json' -d '{"currentTrackId":"t2","position":45.2,"state":"playing"}'
curl -X DELETE "http://host:4533/api/v1/jam/<id>" -H 'Authorization: Bearer <host-token>'
```

Only the host can change playback state or end the session (`403` otherwise).

## Playlist import

```bash
curl "http://host:4533/api/v1/imports/sources" -H 'Authorization: Bearer <token>'

curl -X POST "http://host:4533/api/v1/imports" -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{"source":"deezer","ref":"https://www.deezer.com/playlist/1234567890"}'
# → { "id": "<importId>", "status": "queued", ... }

curl "http://host:4533/api/v1/imports/<importId>" -H 'Authorization: Bearer <token>'  # progress + items[]
curl "http://host:4533/api/v1/imports" -H 'Authorization: Bearer <token>'             # list, no items

# resolve a doubtful/missing/failed item — no body validates the flagged candidate as-is
curl -X POST "http://host:4533/api/v1/imports/<importId>/items/<itemId>/resolve" -H 'Authorization: Bearer <token>'
# with a body, re-searches and uses the best match instead
curl -X POST "http://host:4533/api/v1/imports/<importId>/items/<itemId>/resolve" \
  -H 'Authorization: Bearer <token>' -H 'Content-Type: application/json' \
  -d '{"query":"Radiohead Karma Police"}'
```

## Federation (admin)

```bash
# 1. point at your hub account
curl -X POST "http://host:4533/api/v1/admin/settings" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' -d '{"federation":{"userId":"<your-hub-user-uuid>"}}'

# 2. link — the server does the whole exchange with the hub and persists the issued identity
curl -X POST "http://host:4533/api/v1/admin/federation/register" -H 'Authorization: Bearer <admin>'

curl "http://host:4533/api/v1/admin/federation" -H 'Authorization: Bearer <admin>'          # profile
curl -X PATCH "http://host:4533/api/v1/admin/federation" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' -d '{"name":"My instance","sqid":"my-instance"}'
curl -X DELETE "http://host:4533/api/v1/admin/federation" -H 'Authorization: Bearer <admin>' # unlink

curl "http://host:4533/api/v1/admin/federation/instances?query=" -H 'Authorization: Bearer <admin>'
curl "http://host:4533/api/v1/admin/federation/subscriptions" -H 'Authorization: Bearer <admin>'
curl -X POST "http://host:4533/api/v1/admin/federation/subscriptions" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' -d '{"instanceId":"..."}'
curl -X DELETE "http://host:4533/api/v1/admin/federation/subscriptions/<id>" -H 'Authorization: Bearer <admin>'
```

Toggle `syncPlaylists`/`exportScrobbles` the same way as any other runtime
setting, under `{"federation": {...}}` — all hot, no restart.

## Concert discovery (admin)

See [Configuration](../configuration.md#concert-discovery) for what each field
means and which sources cover which countries.

```bash
curl "http://host:4533/api/v1/admin/concerts" -H 'Authorization: Bearer <admin>'
curl -X PUT "http://host:4533/api/v1/admin/concerts" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' \
  -d '{"enabled":true,"country":"FR","ticketmasterApiKey":"...","skiddleApiKey":"..."}'

# force an immediate sync instead of waiting for the daily one
curl -X POST "http://host:4533/api/v1/admin/concerts/sync" -H 'Authorization: Bearer <admin>'
```

API keys are write-only — `GET /admin/concerts` reports `ticketmasterConfigured`/
`skiddleConfigured` booleans, never the keys themselves.

```bash
# a user's own upcoming, non-dismissed matches, soonest first
curl "http://host:4533/api/v1/me/concerts" -H 'Authorization: Bearer <token>'
curl -X PUT "http://host:4533/api/v1/me/concerts/<id>/dismiss" -H 'Authorization: Bearer <token>'
```

## UI theme

Each account stores its own theme (currently just an accent colour), applied
client-side and persisted server-side so it follows the user across devices.

```bash
curl "http://host:4533/api/v1/theme" -H 'Authorization: Bearer <token>'
# → { "theme": { "accentColor": "#3b82f6" } }

curl -X POST "http://host:4533/api/v1/theme" -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' -d '{"accentColor":"#3b82f6"}'
```

`POST` is a partial update — an empty `accentColor` clears it. Invalid colours
(anything that isn't a CSS hex `#RGB`/`#RRGGBB`/`#RRGGBBAA`) are rejected with
`400`.
