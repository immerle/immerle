---
sidebar_position: 10
title: Purchase import
---

# Purchase import

Bring music you've actually bought on another store into your library.
Purchase import is **source-pluggable** — Bandcamp ships first; Qobuz is
planned as a follow-up (it needs a different, more fragile auth scheme).

Unlike [playlist import](./playlist-import.md), this downloads the real
audio file you paid for — there's no matching against on-demand providers,
since providers like Jamendo or Internet Archive aren't the same music as
what you bought.

## Sources

| Source | Auth | Status |
| ------ | ---- | ------ |
| Bandcamp | Your personal session cookie (pasted by you — Bandcamp has no OAuth for this) | Available |
| Qobuz | A private app id/secret embedded in their apps | Planned |

## Connecting Bandcamp

Bandcamp doesn't offer an API key or OAuth flow for reading your own
collection, so you connect by pasting your browser's session cookie:

1. Log into [bandcamp.com](https://bandcamp.com) in a desktop browser.
2. Open dev tools → Application (Chrome) or Storage (Firefox) → Cookies →
   `https://bandcamp.com`.
3. Copy the value of the cookie named `identity`.
4. Paste it into Settings → Bandcamp purchases → Connect.

Your instance validates the cookie immediately (a bad or expired cookie is
rejected on the spot) and stores it **encrypted at rest** — the same
mechanism the server already uses to store legacy Subsonic passwords
reversibly. It is never returned by any API response.

The cookie is a real, live login session — anyone with both your database
and your instance's config secret could decrypt and use it. Treat it like a
password, and disconnect if you ever suspect it's been exposed.

Sessions aren't refreshable: if you log out everywhere, change your
password, or Bandcamp otherwise invalidates it, the next import attempt
fails and your connection is flagged "needs reconnect" — just paste a fresh
cookie.

## How import works

Opening the Bandcamp screen fetches your purchase collection live (nothing
is cached or synced in the background). Tapping **Import** on an album or
track queues a background job:

1. The exact download link is resolved fresh at the moment the job runs
   (Bandcamp has no way to look up a single purchased item, so this re-lists
   your collection — expect it to take a little longer for large libraries).
2. The best available format is picked automatically, in order: FLAC,
   MP3 320, AAC, ALAC, MP3 V0, Vorbis, WAV, AIFF. There's no format picker.
3. The file (a zip for an album, a single file for a track) downloads and is
   extracted, then ingested exactly like a manual upload — same tag reading,
   same per-user library. Bandcamp's own downloads are already well-tagged,
   so no extra identification step is needed.

Imports run one at a time, in the background — re-tapping Import on an
already-queued or already-imported item is a no-op rather than a duplicate.

---

For the exact API calls behind all of this, see the
[native API walkthrough](./developers/api-guide.md).
