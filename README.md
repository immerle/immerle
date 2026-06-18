# Immerle

> A self-hosted music server with a song of its own.

**Immerle** *(pronounced “I-mmerle”)* speaks the **Subsonic / OpenSubsonic** API,
so it works out of the box with the clients you already use — Supersonic,
Symfonium, DSub, and the rest — and then adds the social, on-demand and
federated features Subsonic never had: friends, an activity feed, collaborative
playlists, synchronized *Jam* listening sessions, an on-demand catalog, playlist
import, and optional federation with an `immerle-hub`.

It ships as a **single Go binary**, runs on **SQLite by default** (Postgres
optional), is bootstrap-configured via environment / `.env`, and is managed at
runtime through an admin API.

### Why “Immerle”?

A small wink at [Immich](https://immich.app), the self-hosted photo server whose
own name nobody has ever quite explained — and *merle*, French for **blackbird**,
the songbird famous for its beautiful improvised whistling. A self-hosted server,
for music, that sings: **Immerle**.

---

## Features

- **Subsonic / OpenSubsonic compatible** — browsing, search, streaming,
  transcoding, playlists, scrobbling, now-playing. Use any existing client.
- **On-demand catalog** — pluggable providers (Jamendo, Internet Archive, and
  runtime-registered HTTP providers) stream and ingest tracks you don't own yet,
  progressively on first play.
- **Social** — friends, activity feed with per-event privacy, collaborative &
  public/subscribable playlists, shares.
- **Jam sessions** — synchronized listening, streamed over SSE.
- **Playlist import** — pluggable sources (Spotify ships first), matched against
  the on-demand providers.
- **Federation (opt-in)** — editorial/reco playlist sync and portable-id
  resolution via an `immerle-hub`.
- **Auth** — Subsonic token/password, device JWTs (revocable), and personal API
  tokens.
- **OpenAPI 3.1** spec + self-contained Swagger UI for the native API.

## Contents

- [Quick start](#quick-start)
- [First-run setup](#first-run-setup)
- [Configuration](#configuration)
- [Architecture](#architecture)
- [The native Immerle API](#the-native-immerle-api)
- [On-demand providers & artist avatars](#on-demand-providers--artist-avatars)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## Quick start

### Docker

```bash
# put your music under ./music, then:
docker compose up --build
# server on http://localhost:4533 — create the admin via the first-run setup:
curl -X POST http://localhost:4533/setup/init \
  -H 'Content-Type: application/json' \
  -d '{"username":"me","password":"a-strong-password"}'
```

### From source

```bash
make build
cp .env.example .env   # edit as needed
./bin/immerle          # auto-loads .env (or pass -env path/to/.env)
```

Requirements: Go 1.25+, and `ffmpeg`/`ffprobe` on `PATH` for transcoding,
duration probing and on-demand tag embedding.

### Health & connecting a client

- Health check: `GET /ping` → `{"status":"ok"}`
- After setup, point a Subsonic client at `http://<host>:4533` with the
  credentials you just created.

## First-run setup

There is **no** admin account provisioned from config or environment. On a fresh
database the server starts in *setup mode*; create the first administrator
through the setup API (the only bootstrap path):

```bash
# Is setup needed?
curl http://localhost:4533/setup/status

# Create the first admin (locks itself afterwards)
curl -X POST http://localhost:4533/setup/init \
  -H 'Content-Type: application/json' \
  -d '{"username":"me","password":"a-strong-password","email":"me@example.com","displayName":"Me"}'
```

`displayName` is an **optional** free-text name shown in the UI in place of the
login username (it falls back to the username when empty). Set
`AUTH_REQUIRE_SETUP_TOKEN=true` to gate setup behind a one-time token printed
at startup (logs and `data/setup-token`); pass it as `setupToken` in the init
request.

## Configuration

Configuration is split in two:

**Bootstrap (environment / `.env`, restart-required).** A small set that must be
known before startup: server port, database, logging, library paths, the
first-run setup-token gate and built-in provider credentials. Copy
[`.env.example`](.env.example) to `.env` (auto-loaded at startup) or set the
variables in the environment. Variables are plain — `PORT`, `DATABASE_DSN`,
`LIBRARY_PATHS`, … (see [`.env.example`](.env.example)). **Real environment
variables take precedence over the `.env` file.** `PORT` is a bare number (e.g.
`PORT=4533`); scan-on-start is always on; the auth secret is **auto-generated and
stored in `data/configuration.yaml`** on first run (override with `AUTH_SECRET`
only if you need a specific value).

**Runtime (admin API, hot or restart).** Everything else an operator changes
while running lives in **`data/configuration.yaml`** (a human-readable file,
which also holds the secret) and is managed via the admin API — not in `.env`:

| Group | Endpoint | Reload |
| ----- | -------- | ------ |
| CORS allowed origins (default `*`) | `GET/POST /admin/settings` | **hot** |
| Device-token TTL | `GET/POST /admin/settings` | **hot** |
| Provider behaviour (default, auto-download, search timeout) | `GET/POST /admin/settings` | **hot** |
| Scan interval | `GET/POST /admin/settings` | **hot** |
| Transcoding (ffmpeg/ffprobe paths, profiles) | `GET/POST /admin/settings` | restart |
| Artist avatars (enabled, source) | `GET/POST /admin/settings` | restart |
| Scan watcher (fsnotify on/off) | `GET/POST /admin/settings` | restart |
| Federation (enabled, hub URL/keys, interval, flags) | `GET/POST /admin/settings` | **hot** |
| On-demand providers (CRUD, order, enable) | `/admin/providers*` | **hot** |
| Provider-download cleanup | `/admin/cleanup*` | **hot** |

`GET /admin/settings` returns the current values; `POST` applies a **partial**
update (JSON body). When a change only takes effect after a restart, the response
sets `"restartRequired": true` and lists the fields in `pendingRestart` so the UI
can prompt the operator to restart. Hot changes apply immediately.

```bash
curl "http://host:4533/admin/settings?u=admin&p=pw&c=app"
# hot: tune provider behaviour (applies now)
curl -X POST "http://host:4533/admin/settings?u=admin&p=pw&c=app" \
  -H 'Content-Type: application/json' \
  -d '{"providers":{"autoDownloadOnPlay":true,"searchTimeoutSeconds":8}}'
# hot: enable federation / point at a hub (applies on the next sync tick)
curl -X POST "http://host:4533/admin/settings?u=admin&p=pw&c=app" \
  -H 'Content-Type: application/json' \
  -d '{"federation":{"enabled":true,"hubUrl":"https://hub.example","publicKey":"…","privateKey":"…"}}'
# restart-required: toggling the scan watcher → response has restartRequired:true
curl -X POST "http://host:4533/admin/settings?u=admin&p=pw&c=app" \
  -H 'Content-Type: application/json' \
  -d '{"scan":{"watch":false}}'
```

### Database

SQLite by default. For large instances:

```bash
DATABASE_DRIVER=postgres
DATABASE_DSN=postgres://immerle:immerle@localhost:5432/immerle?sslmode=disable
```

Migrations (goose, embedded) apply automatically at startup.

## Architecture

Layered, with clear boundaries:

```
cmd/immerle              entrypoint
internal/
  config                 bootstrap config (.env / environment)
  logging                structured logging (slog)
  db                     connection pool, goose migrations, dialect helpers
  models                 domain entities
  persistence            repositories (one per aggregate) over database/sql
  scanner                filesystem walk, tag extraction, idempotent upserts
  stream                 audio streaming (range/seek), ffmpeg transcoding, cover art
  providers              pluggable on-demand catalog providers (jamendo, internet-archive, http)
  core                   business services (auth, annotations, on-demand,
                         activity, jam, now-playing)
  federation             immerle-hub client
  api/subsonic           Subsonic / OpenSubsonic handlers (XML + JSON)
  api/immerle            native immerle extension handlers (JSON + SSE)
  server                 HTTP server with graceful shutdown
  app                    wiring
```

### Milestones

| Milestone | Feature |
|-----------|---------|
| S0 | Foundations: config, logging, graceful shutdown, DB pool, migrations, `/ping`, CI, Docker |
| S1 | Scanner: recursive walk, tag extraction (dhowden/tag + ffprobe), idempotent dedup, full + incremental (fsnotify/periodic) scans, rename-safe identity (MBID/hash) |
| S2 | Subsonic browsing & search: auth (token/password), XML+JSON, `getArtists`/`getArtist`/`getAlbum`/`getAlbumList2`/`getSong`/`getGenres`/`getIndexes`/`getMusicFolders`, `search3`, `getCoverArt` (resize + cache), OpenSubsonic extensions |
| S3 | Streaming & transcoding: `stream` (Range/seek) + `download`, ffmpeg profiles by `maxBitRate`/`format`, transcode cache, no leaked ffmpeg processes |
| S4 | Multi-user: accounts (admin/non-admin), per-user star/rating/playcount, `scrobble`, `getNowPlaying`, playlists CRUD, server `get/savePlayQueue` |
| S5 | On-demand catalog: pluggable `Provider` interface, async `download_jobs` queue with resume, download→tag→file layout→scan ingest, hooks in `search3` and streaming, strict MBID/hash dedup |
| S6 | immerle social: `immerle.capabilities` discovery, collaborative playlists, friends, activity feed with privacy, shares, Jam sessions (SSE) |
| S7 | Hub federation: opt-in registration, editorial/reco playlist sync, portable-id resolution (+ optional on-demand download), anonymized scrobble export, federated read-only playlists |

## The native Immerle API

Capability discovery is unauthenticated so apps can detect support:

```bash
curl http://localhost:4533/capabilities
```

Other endpoints (`/friends`, `/activity`, `/profile`, `/jam/*`, …) authenticate
with the same credentials as Subsonic (`u` + `p`, or `u` + `t` + `s`). Jam
sessions stream state over SSE at `/jam/events`.

`GET /profile?username=<name>` returns a user's profile — identity
(`username`, `displayName`, `isAdmin`), their recent **activity** visible to the
caller (honoring each event's privacy), their **public playlists**, and
`isSelf`/`isFriend` flags. Omit `username` to fetch your own profile. Activity
events (in `/activity` and `/profile`) carry the author's `displayName`
alongside `username`.

`/account` is the caller's **own editable account**: `GET /account` returns it
(including the private `email`, which public profiles never expose), and
`POST /account` applies a partial self-update of `displayName` and `email` (only
fields present in the request change; an empty value clears one). So any user can
set their own display name and email — no admin needed. (Admins can still set
these for anyone via Subsonic `createUser`/`updateUser`.)

`GET /library/stats` returns the **library analytics**: `artists`, `albums`,
`tracks`, `totalSize` (bytes on disk) and `totalDuration` (seconds), plus the
`updatedAt` of the snapshot. The snapshot is **computed at each scan** and cached
in memory, so the endpoint never sums over every track on request (no per-request
I/O). `totalSize` is the sum of the indexed files' on-disk sizes recorded during
the scan.

### Public playlists & subscriptions

An owner makes a playlist public (`updatePlaylist?public=true`). Other users
don't see every public playlist in their library; they **subscribe** to opt in:

```bash
# discover public playlists
curl "http://host:4533/playlists/public?u=me&p=pw&c=app"
# subscribe → it then appears in getPlaylists like a normal (read-only) playlist
curl -X POST "http://host:4533/playlists/subscribe?u=me&p=pw&c=app&playlistId=<id>"
# unsubscribe
curl -X POST "http://host:4533/playlists/unsubscribe?u=me&p=pw&c=app&playlistId=<id>"
```

A subscriber **cannot modify** the playlist (edits are refused). In a Subsonic
client, "deleting" a subscribed playlist simply **unsubscribes** (removes it from
your library) — the owner's playlist is untouched. `getPlaylists` returns your
own + collaborative + subscribed + federated playlists.

For a richer list UI, every playlist carries a **`coverArts`** array (immerle
extension) — the cover-art ids of its **first up-to-4 tracks**, in order, for a
mosaic thumbnail (a track's cover falls back to its album when it has none). It
is computed in a single set-based query per list (no per-playlist round-trip) and
appears on Subsonic `getPlaylists`/`getPlaylist` as well as the immerle
`/playlists/public` and `/profile` playlist entries.

### Playlist import (Spotify, pluggable)

Import a playlist from an external service into a new immerle playlist. The
feature is **source-pluggable** (an `importer.Source` interface + factory
registry, mirroring the content providers) — **Spotify** ships first; add a
source by registering a factory, no engine changes.

Three concepts are kept **distinct** so a dedicated imports UI can show progress:
the **import job** (the operation + per-track status), the **source listing**
(the playlist as it exists at Spotify), and the **immerle playlist** that gets
created. The import job carries the link to the created playlist plus running
counters.

How it works: a background worker fetches the source playlist (title + tracks),
creates the destination playlist, seeds one **import item** per source track
(`pending`), then for each track searches the **on-demand content providers** by
`artist + title`, picks the best candidate and scores it with a normalized
string similarity (Levenshtein). Per-track outcome:

- **matched** — similarity ≥ 90%: the track is downloaded/ingested and appended
  to the playlist (`matchedTrackId` set).
- **doubtful** — a candidate was found but below 90%: recorded with its
  confidence and the resolved title/artist, **not** added (left for review).
- **missing** — no candidate at any provider.
- **failed** — a search or download error (recorded in `note`).

| Method | Endpoint | Description |
| ------ | -------- | ----------- |
| `GET`  | `/imports/sources` | List import sources and whether each is configured. |
| `POST` | `/imports/start` | Queue an import (`source`, `ref` = playlist id/URL). Returns the job. |
| `GET`  | `/imports` | List the caller's imports (no items). |
| `GET`  | `/imports/status?id=` | One import with its per-track items (the progress view). |
| `POST` | `/imports/items/resolve` | Validate/modify a doubtful (or missing/failed) item: download a track and add it to the playlist. |

A **doubtful** item (or a missing/failed one) can be resolved from the imports
page via `POST /imports/items/resolve?itemId=…`: with no `query` it **validates**
the flagged candidate as-is (downloads it and adds it to the playlist); with a
`query` ("artist title") it **modifies** the match — re-searching the content
providers and using the best result. Either way the item flips to `matched`, the
track is appended to the import's playlist, and the import counters rebalance.

**Spotify goes through the hub**, not the Spotify API directly: the source
delegates to the federation hub's lazy import job — `POST {hub}/api/v1/spotify/imports`
then poll `GET {hub}/api/v1/spotify/imports/{id}` until completed — so **no
Spotify credentials live on the instance** — the hub holds them. It therefore
requires a configured hub: set `federation.hubUrl` plus the hub-issued
`federation.publicKey` / `federation.privateKey` (sent as `X-Instance-ID` and
`Authorization: Bearer`) in the runtime settings — all **hot-reloadable** (no
restart). The hub client is always running and reads its config live, so just
setting a hub URL makes import available even with background federation **sync**
left disabled. The `ref` accepts a playlist id, a `spotify:playlist:…` URI, or an
`open.spotify.com/playlist/…` URL (the hub parses it). A future source that
authenticates directly instead can still use `import.sources.<name>` for its own
config.

### Device sessions (JWT)

A client logs in and gets a **JWT** carrying a unique id (`jti`). Each JWT is
recorded in a **devices registry** that doubles as the revocation list and the
last-seen tracker — so every login is a uniquely identifiable, revocable device.

```bash
# log in → a device JWT (store it; send as Authorization: Bearer)
curl -X POST "http://host:4533/auth/login?u=me&p=pw&c=app&device=MacBook"
# → { "ok": true, "token": "eyJ…", "device": { "id": "<jti>", ... } }

curl -H "Authorization: Bearer eyJ…" "http://host:4533/rest/getArtists?c=app&f=json"

# see / revoke your devices
curl -H "Authorization: Bearer eyJ…" "http://host:4533/devices?c=app"
curl -X POST "http://host:4533/devices/revoke?u=me&p=pw&c=app&id=<jti>"   # JWT dies
```

JWTs are HS256-signed with a key derived from the auth secret (auto-generated and
stored in `data/configuration.yaml`, or `AUTH_SECRET`); verification is stateless
except for one indexed check that the `jti` isn't revoked (which also refreshes
last-seen/IP). Lifetime is the runtime setting `auth.deviceTokenTtlSeconds`
(`POST /admin/settings`; default 30 days, `0` = never, revoke-only).

### Personal API tokens

Users can mint personal access tokens (scoped to themselves) to authenticate API
requests without their password:

```bash
# create (returns the secret ONCE)
curl -X POST "http://host:4533/tokens/create?u=me&p=pw&c=app&name=my-cli"
# → { "ok": true, "token": "gsk_…", "id": "…", "prefix": "gsk_…" }

# use it — as a Bearer header or ?apiKey, on BOTH the Subsonic and immerle APIs
curl -H "Authorization: Bearer gsk_…" "http://host:4533/rest/getArtists?c=app&f=json"
curl "http://host:4533/rest/getArtists?c=app&f=json&apiKey=gsk_…"

# list / revoke
curl "http://host:4533/tokens?u=me&p=pw&c=app"
curl -X POST "http://host:4533/tokens/revoke?u=me&p=pw&c=app&id=<tokenId>"
```

Only a SHA-256 hash of the token is stored; the secret is shown once. A token
authenticates as its creating user; tokens are listed/revoked per owner and can
carry an optional expiry.

### UI theme

Each user stores a per-account UI theme (applied client-side), persisted as JSON
on the `users` row so new properties can be added without a schema change. Only
the **accent colour** is supported for now.

```bash
# read the caller's theme
curl "http://host:4533/theme?u=me&p=pw&c=app"
# → { "ok": true, "theme": { "accentColor": "#3b82f6" } }

# set the accent colour (CSS hex: #RGB, #RRGGBB or #RRGGBBAA)
curl -X POST "http://host:4533/theme?u=me&p=pw&c=app&accentColor=%233b82f6"
# clear it (empty value)
curl -X POST "http://host:4533/theme?u=me&p=pw&c=app&accentColor="
```

POST is a **partial update** — only fields present in the request change.
Invalid colours are rejected with HTTP 400.

### OpenAPI / Swagger

The native immerle API is documented with an **OpenAPI 3.1** specification,
generated from handler annotations with [swaggo/swag v2](https://github.com/swaggo/swag)
and served by the binary:

- `GET /openapi.json` / `GET /openapi.yaml` — the spec
- `GET /swagger/` — interactive Swagger UI (self-contained, no CDN)

Regenerate after changing annotations (and keep it committed — CI enforces it):

```bash
make openapi        # regenerate internal/api/docs/swagger.{json,yaml}
make openapi-check  # fail if the committed spec is stale
```

The Subsonic/OpenSubsonic surface under `/rest/` follows the published
[OpenSubsonic spec](https://opensubsonic.netlify.app/) and is not duplicated here.

## On-demand providers & artist avatars

The on-demand catalog (S5) is always running — with no enabled provider it simply
has nothing to search/download (equivalent to "off"). **All** providers —
built-in and dynamic — are configured through the admin API (`/admin/providers`):
each has a **JSON config**, and a built-in's credentials live in that config (no
env vars). Adding/editing/enabling/reordering a provider is applied **hot** (live
registry + DB updated together, no restart). Provider **behaviour** (default
provider, auto-download, search timeout) is also a hot runtime setting.

Shipped built-in providers (legal, no DRM): **`jamendo`** (Creative Commons
catalog, free authorized downloads — seeded **disabled** with a
`{"client_id":"<JAMENDO_TOKEN>","audioformat":"mp32"}` config to fill in and
enable) and **`internet-archive`** (archive.org: public-domain recordings,
Creative Commons works and artist-sanctioned live music — no credentials, no DRM).

Other catalogs are added **at runtime** as external services rather than compiled
in (see *Dynamic providers* below). For example, **Deezer metadata** lives in a
standalone `deezer-http` module — a separate, dependency-free service that
exposes Deezer's *public* catalog over the provider protocol (search/resolve,
**no** download) — and is registered via the admin API. The core ships **no**
Deezer downloader.

### Dynamic providers (runtime, admin-managed)

Beyond the compile-time factories, an **admin** can register **content-neutral
HTTP providers at runtime** — no restart, no rebuild. A dynamic provider is just
a **name**, an **HTTP endpoint** and an opaque **JSON config**; the core calls
that endpoint for search/resolve/download and neither knows nor cares what's
behind it. This is the seam for plugging in any out-of-process catalog or
downloader you operate and have the rights to use. The admin endpoints return
**403** for non-admins.

The same API also surfaces the **built-in** providers (compiled-in factories).
You can **edit their JSON config** (e.g. set a credential), **disable** and
**reorder** them — but **not delete** them. Their credentials live in the config
JSON, edited via the same `POST /admin/providers`.

| Method | Endpoint | Description |
| ------ | -------- | ----------- |
| `GET`  | `/admin/providers` | List all providers (built-in + dynamic) with `enabled`, `active`, `builtin`, `deletable`, `sortOrder`. |
| `POST` | `/admin/providers` | Create/update a dynamic provider, or edit a built-in's `config`/`enabled` (`name`, `endpoint`, `config`, `enabled`). |
| `POST` | `/admin/providers/enable` | Toggle any provider (`name`, `enabled`). |
| `POST` | `/admin/providers/reorder` | Set priority (`order` = comma-separated names, each once). |
| `POST` | `/admin/providers/delete` | Remove a **dynamic** provider (built-ins → 400). |

```bash
# create/update a provider named "manual" (enabled & registered immediately)
curl -X POST "http://host:4533/admin/providers?u=admin&p=pw&c=app" \
  --data-urlencode name=manual \
  --data-urlencode endpoint=https://my-service.internal \
  --data-urlencode 'config={"headers":{"Authorization":"Bearer xxx"}}'

curl     "http://host:4533/admin/providers?u=admin&p=pw&c=app"                       # list
curl -X POST "http://host:4533/admin/providers/enable?u=admin&p=pw&c=app&name=jamendo&enabled=false"
curl -X POST "http://host:4533/admin/providers/reorder?u=admin&p=pw&c=app&order=manual,jamendo,deezer"
curl -X POST "http://host:4533/admin/providers/delete?u=admin&p=pw&c=app&name=manual"
```

Config and order are persisted (`provider_configs`) and reloaded on boot; an
enabled config is live in the registry, a disabled one is removed. **Order is the
priority** — the first enabled provider is the one search/enrichment uses (there
is no separate "default" setting). A **newly added provider is placed first**
(highest priority) so it takes effect immediately without a manual reorder;
editing an existing provider keeps its position. Dynamic names must be slugs and
may not shadow a built-in. The remote service implements three endpoints (paths
configurable): `GET {endpoint}/search?q=&limit=` → `{"results":[<track>]}`,
`GET {endpoint}/resolve?id=` → `<track>`, and `GET {endpoint}/download?id=` → raw
audio bytes. See the OpenAPI spec and `internal/providers/httpprovider` for the
exact `<track>` shape.

A download whose **open phase** fails transiently (network error, or a non-2xx
status — e.g. the remote momentarily failing to mint a token) is **retried**
(`downloadRetries`, default **3**, with a short linear backoff). Retries only
cover the pre-stream phase: once a 2xx body is acquired and audio bytes start
flowing they are never replayed, so a mid-stream error fails the call.

### Progressive first play

When a Subsonic client streams a **remote** track that isn't local yet, the first
listen is served **progressively**: the provider's bytes are teed to the client
and to disk at the same time (`PrepareStream` + `StreamPending`), so playback
starts immediately instead of waiting for the whole download. That first stream is
the provider's original audio (no transcoding — transcoding would force buffering
the whole file first), advertised with the **requested** format's content type.
The saved copy is then ingested in the background (tags embedded, scanned, a
completed `download_jobs` row recorded), so later plays resolve **locally** and go
through the normal transcoding/seekable path. Concurrent first plays of the same
track each stream independently; a single `singleflight`-guarded finalize wins, so
the file is ingested once. If a track is already local (by MBID, or a prior
completed download), it skips straight to the normal local path.

Search (`search3`/`search2`) **merges** the local library with results from the
**first provider by order** — both **songs and artists** — deduplicated (by id,
against local tracks by MBID and local artists by name). Search targets a single
provider (not a fan-out), runs the song and artist lookups concurrently, caches
provider results (60s TTL, singleflight) and is bounded by the runtime search
timeout (default 6s). Remote artists are browsable: `getArtist`/`getMusicDirectory`
on a remote artist re-query the provider (via its `ArtistAlbumLister`/`AlbumBrowser`
capabilities, e.g. Deezer's artist page) to list the discography and each album's
tracks.

A **local** artist's `getArtist` is also enriched with the rest of the artist's
discography from the provider: albums you don't own are merged in (deduplicated
by album name), browsable, and stream/download on play. So a local artist with
one album shows their full catalog, not just what's on disk.

`getArtist?includeSongs=true` inlines each album's tracks in the response
(`album[].song[]`) so a client can render the whole artist in one call. Off by
default; local album songs come from the catalog, remote ones are fetched from
the provider concurrently (bounded, with a timeout). Remote cover art / avatars
(e.g. Deezer's `ALB_PICTURE`/`ART_PICTURE`) are served through `getCoverArt`,
fetched on demand from the provider's public image CDN and cached. A **host
allowlist** (default `dzcdn.net`) guards against SSRF.

Local cover art is read from **embedded** tags and from **sidecar files**
(`cover.jpg`, `folder.jpg`, `front`, `albumart`, … in the album folder or its
parent), resized and cached.

Artist avatars are fetched **through the on-demand providers** — the same place
artists themselves come from. If a registered provider exposes the artist-image
capability (`GET {endpoint}/artist/image?name=` → `{"imageUrl":"…"}`, e.g.
`deezer-http`), the enricher resolves a candidate URL, downloads it (metadata
only, never audio), validates it really is an image, caches it locally and exposes
it through the standard `coverArt` of artists and `getArtistInfo2` image URLs.
There is **no enable/disable toggle**: enrichment is active whenever a provider
can supply images and idles otherwise.

### Cleanup of unused provider downloads

Tracks pulled in by a provider can be garbage-collected so on-demand downloads do
not accumulate forever. A downloaded track is deleted (its file and DB rows) only
when there is **no reason to keep it**: it has **not been played** within
`max_age` **and** is in **no playlist** **and** is **starred by nobody**.
Manually-added music (anything without a completed download job) is never touched.
The sweep is **on by default** (30-day window, 6h cadence) and managed at runtime
by an admin:

| Method   | Endpoint              | Description                                          |
| -------- | --------------------- | ---------------------------------------------------- |
| `GET`    | `/admin/cleanup`      | Report the sweep state (`enabled`, `maxAge`, `interval`). |
| `POST`   | `/admin/cleanup`      | `enabled=true\|false` — turn the background sweep on/off. |
| `POST`   | `/admin/cleanup/run`  | Run one sweep immediately (works even when disabled); returns `removed`. |

## Development

```bash
make test        # run the suite
make test-race   # with the race detector
make lint        # golangci-lint
make ci          # tidy + vet + lint + test + build
```

Tests that need real audio generate fixtures with `ffmpeg` and skip when it is
not installed.

## Contributing

Issues and pull requests are welcome. Before opening a PR, run `make ci` (it
must pass) and regenerate the OpenAPI spec with `make openapi` if you touched
handler annotations — CI fails on a stale spec.

## License

No license has been declared yet. Until one is added, default copyright applies
and all rights are reserved — open an issue if you'd like to use Immerle in your
own project.
