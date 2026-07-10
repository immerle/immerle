---
sidebar_position: 7
title: Playlist import
---

# Playlist import

Bring a playlist over from another service into a new Immerle playlist.
Import is **source-pluggable** — Spotify and Deezer ship first, more sources
can be added later without changing how import itself works.

The import runs in the background: a job is created immediately, and each
source track is matched to your on-demand catalog one at a time, so a client
can show live progress instead of waiting for the whole playlist.

## Sources

| Source | How it works | Requirements |
| ------ | ------------- | ------------ |
| Spotify | Delegated to the [federation](./federation.md) hub — the hub holds Spotify credentials, not your instance | A configured hub connection |
| Deezer | Fetched directly from Deezer's public API (no auth needed for public playlists) | None — always available |

You can point an import at either a bare playlist id or a full playlist URL
from the source service.

## How a track resolves

Each track in the source playlist is searched for across your enabled
on-demand providers and scored by how closely the result matches. Depending
on the outcome, a track ends up in one of four states:

- **Matched** — a high-confidence result was found, downloaded and added to
  the playlist automatically.
- **Doubtful** — a candidate was found, but the match wasn't confident enough
  to add automatically. It's held for you to review.
- **Missing** — no candidate was found on any provider.
- **Failed** — a search or download error occurred.

Anything that isn't matched can be fixed by hand afterwards: either accept
the flagged candidate as-is, or search again yourself with a different query
and use that result instead. Either way, once resolved the track is
downloaded and appended to the playlist just like a normal match.

---

For the exact API calls behind all of this, see the
[native API walkthrough](./developers/api-guide.md).
