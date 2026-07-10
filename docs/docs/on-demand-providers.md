---
sidebar_position: 6
title: On-demand catalog
---

# On-demand catalog

Beyond your own library, Immerle can search and stream tracks you don't own
yet from pluggable **providers** — legal, no-DRM sources. The first listen
streams progressively while the track is saved in the background; every later
play is local, transcoded and seekable like anything else in your library.

The on-demand catalog is always running: with no provider enabled it simply
has nothing to search, equivalent to being off. There's no separate on/off
switch for the feature itself — you manage it by enabling/disabling
individual providers.

## Built-in providers

Shipped compiled into the binary, configured (never coded) through the admin
API — credentials live in a provider's JSON config, never in `.env`:

| Provider | Catalog | Credentials | Default |
| -------- | ------- | ------------ | ------- |
| `jamendo` | Creative Commons, free authorized downloads | Client ID (`JAMENDO_TOKEN`) required | seeded, **disabled** until configured |
| `internet-archive` | Public domain, CC and artist-sanctioned live recordings | none | enabled |
| `free-music-archive` | freemusicarchive.org CC catalog | none | enabled, first in priority order |

**Order is priority** — the first *enabled* provider is the one search and
enrichment use; there's no separate "default provider" setting.

```bash
# see current providers, priority order, and live status
curl "http://host:4533/api/v1/admin/providers" -H 'Authorization: Bearer <admin>'

# fill in Jamendo's client id and enable it
curl -X POST "http://host:4533/api/v1/admin/providers" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' \
  -d '{"name":"jamendo","config":"{\"params\":{\"client_id\":\"<JAMENDO_TOKEN>\",\"audioformat\":\"mp32\"}}"}'
curl -X PUT "http://host:4533/api/v1/admin/providers/jamendo/enabled" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' -d '{"enabled":true}'
```

## Dynamic (HTTP) providers

Beyond the compiled-in ones, an admin can register **any HTTP service** as a
provider at runtime — no rebuild, no restart. Immerle calls a handful of fixed
JSON-over-HTTP endpoints on it (`/capabilities`, `/search`, `/resolve`,
`/download`) and is otherwise content-neutral: it doesn't know or care what's
behind the URL.

This is the seam for plugging in a catalog you operate yourself — see
[Building a custom content provider](./custom-provider.md) for the exact
protocol if you're writing one. A provider's **name comes from its own
`/capabilities` response**, not from what you type when adding it.

```bash
# add by URL only — name + config skeleton are probed from /capabilities, created disabled
curl -X POST "http://host:4533/api/v1/admin/providers" -H 'Authorization: Bearer <admin>' \
  -H 'Content-Type: application/json' -d '{"endpoint":"https://my-service.internal"}'
```

`GET /admin/providers` reports each provider's `enabled`, `active`, `builtin`,
`deletable` and live `version` (probed in parallel for dynamic providers).
Built-ins can be reconfigured, disabled and reordered like any other provider,
but not deleted.

## Progressive first play

Streaming a remote track for the first time doesn't wait for a full download:
the provider's bytes are teed to the client and to disk simultaneously, so
playback starts immediately. That first stream is the provider's original
audio (untranscoded — transcoding needs the whole file buffered first). The
saved copy is tagged and scanned into your library in the background, so the
next play resolves locally through the normal transcoding/seekable path.

Search (`search3`) merges local results with the top-priority enabled
provider's results — both songs and artists, deduplicated — and a local
artist's page is enriched with the rest of their discography from that
provider, browsable and streamable on demand.

## Cleanup of unused downloads

Provider-downloaded tracks don't accumulate forever. A background sweep
(on by default: 30-day window, every 6h) deletes a downloaded track's file and
DB rows only when there's **no reason to keep it** — unplayed within the
window, in no playlist, starred by nobody. Anything you added manually
(no completed download job behind it) is never touched.

```bash
GET  /api/v1/admin/cleanup       # current state: enabled, maxAge, interval
POST /api/v1/admin/cleanup       # {"enabled": true|false}
POST /api/v1/admin/cleanup/run   # run one sweep now, returns { "removed": <n> }
```

See the [API reference](pathname:///api/) for exact request/response shapes.
