---
sidebar_position: 9
title: Playlist import
---

# Playlist import

Bring a playlist over from another service into a new Immerle playlist.
Import is **source-pluggable** — Spotify and Deezer ship first, more sources
can be added without changing the import engine.

The import runs in the background: a job is created immediately, and each
source track is matched to your on-demand catalog one at a time so a client
can show live progress.

## Sources

| Source | How it works | Requirements |
| ------ | ------------- | ------------ |
| **Spotify** | Delegated to the [federation](./federation.md) hub — the hub holds Spotify credentials, not your instance | A configured hub connection |
| **Deezer** | Fetched directly from Deezer's public API (no auth needed for public playlists) | None — always available |

```bash
# check what's configured/available
curl "http://host:4533/api/v1/imports/sources" -H 'Authorization: Bearer <token>'
```

`ref` accepts either a bare playlist id or a full URL: for Spotify a
`spotify:playlist:…` URI or an `open.spotify.com/playlist/…` link; for Deezer
a bare id or `deezer.com/playlist/…` link (short links aren't resolved — paste
the full URL).

## Running an import

```bash
curl -X POST "http://host:4533/api/v1/imports" -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{"source":"deezer","ref":"https://www.deezer.com/playlist/1234567890"}'
# → { "id": "<importId>", "status": "queued", ... }

# poll for progress, including per-track items
curl "http://host:4533/api/v1/imports/<importId>" -H 'Authorization: Bearer <token>'

# list your imports (summary only, no items)
curl "http://host:4533/api/v1/imports" -H 'Authorization: Bearer <token>'
```

An import's `status` is one of `queued`, `running`, `completed`, `failed`.
Each track inside it (`items[]`) resolves independently to one of:

| Item status | Meaning |
| ----------- | ------- |
| `matched` | found with high confidence (≥ 90% title/artist similarity), downloaded and added to the playlist |
| `doubtful` | a candidate was found below the confidence threshold — held for review |
| `missing` | no candidate found on any provider |
| `failed` | a search or download error occurred |

## Resolving doubtful or missing tracks

Anything not `matched` can be fixed up by hand:

```bash
# accept the flagged candidate as-is
curl -X POST "http://host:4533/api/v1/imports/<importId>/items/<itemId>/resolve" \
  -H 'Authorization: Bearer <token>'

# or search again with a different query
curl -X POST "http://host:4533/api/v1/imports/<importId>/items/<itemId>/resolve" \
  -H 'Authorization: Bearer <token>' -H 'Content-Type: application/json' \
  -d '{"query":"Radiohead Karma Police"}'
```

Either way, a successful resolve downloads the track, appends it to the
import's playlist and flips the item to `matched`.
