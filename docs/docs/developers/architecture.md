---
sidebar_position: 1
title: Architecture & development
---

# Architecture & development

Immerle is a single Go binary with a layered internal structure and clear
boundaries between them:

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
  providers              pluggable on-demand catalog providers (jamendo, internet-archive, free-music-archive, http)
  core                   business services (auth, annotations, on-demand,
                         activity, jam, now-playing)
  federation             immerle-hub client
  api/subsonic           Subsonic / OpenSubsonic handlers (XML + JSON)
  api/immerle            native immerle extension handlers (JSON + SSE)
  server                 HTTP server with graceful shutdown
  app                    wiring
```

## How it grew

| Milestone | Feature |
|-----------|---------|
| S0 | Foundations: config, logging, graceful shutdown, DB pool, migrations, `/ping`, CI, Docker |
| S1 | Scanner: recursive walk, tag extraction (dhowden/tag + ffprobe), idempotent dedup, full + incremental (fsnotify/periodic) scans, rename-safe identity (MBID/hash) |
| S2 | Subsonic browsing & search: auth (token/password), XML+JSON, `getArtists`/`getArtist`/`getAlbum`/`getAlbumList2`/`getSong`/`getGenres`/`getIndexes`/`getMusicFolders`, `search3`, `getCoverArt` (resize + cache), OpenSubsonic extensions |
| S3 | Streaming & transcoding: `stream` (Range/seek) + `download`, ffmpeg profiles by `maxBitRate`/`format`, transcode cache, no leaked ffmpeg processes |
| S4 | Multi-user: accounts (admin/non-admin), per-user star/rating/playcount, `scrobble`, `getNowPlaying`, playlists CRUD, server `get/savePlayQueue` |
| S5 | On-demand catalog: pluggable `Provider` interface, async `download_jobs` queue with resume, download→tag→file layout→scan ingest, hooks in `search3` and streaming, strict MBID/hash dedup |
| S6 | Immerle social: capability discovery, collaborative playlists, activity feed with privacy, shares, Jam sessions (SSE) |
| S7 | Hub federation: opt-in registration, editorial/reco playlist sync, portable-id resolution (+ optional on-demand download), anonymized scrobble export, federated read-only playlists |

## The native Immerle API

Everything under `/api/v1/*` — accounts, social, providers, admin — is
Immerle's own extension on top of the Subsonic surface, documented as an
**OpenAPI 3.1** specification generated straight from handler annotations
with [swaggo/swag v2](https://github.com/swaggo/swag):

- `GET /openapi.json` / `GET /openapi.yaml` — the spec
- `GET /swagger/` — interactive Swagger UI, served by the binary, no CDN
- [API reference](pathname:///api/) — the same spec, browsable from this site

Capability discovery is unauthenticated so client apps can detect what a
server supports before logging in:

```bash
curl http://localhost:4533/api/v1/capabilities
```

Everything else authenticates the same way as Subsonic — `u`+`p`, `u`+`t`+`s`,
a device JWT (`Authorization: Bearer`, from `POST /auth/sessions`), or a
personal API token (`gsk_…`, also a Bearer or `?apiKey=`). See
[Subsonic API](./subsonic-api.md) for the parameter reference and
[On-demand catalog](../on-demand-providers.md) /
[Social features](../social.md) / [Playlist import](../playlist-import.md) /
[Federation](../federation.md) for what each area of the native API covers —
those pages describe the *behavior*; regenerate/consult the OpenAPI spec for
exact request/response shapes.

Regenerate the spec after changing handler annotations (and keep it
committed — CI enforces this):

```bash
make openapi        # regenerate internal/api/docs/swagger.{json,yaml}
make openapi-check   # fail if the committed spec is stale
```

## Building and testing

```bash
make build       # compile ./bin/immerle
make test         # run the suite
make test-race    # with the race detector
make lint         # golangci-lint
make ci           # tidy + vet + lint + test + build
```

Tests that need real audio generate fixtures with `ffmpeg` and skip when it's
not installed. Migrations (goose, embedded in the binary) apply automatically
at startup; SQLite is the default backend, Postgres is supported for larger
instances (`DATABASE_DRIVER=postgres`).

Before opening a PR, run `make ci` (it must pass) and `make openapi` if you
touched handler annotations — CI fails on a stale spec.
