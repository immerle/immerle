---
slug: /
sidebar_position: 1
title: Introduction
---

# Immerle

**Your music, self-hosted — and it sings.**

Immerle is a self-hosted music server with its own **app** (web, iOS, Android)
and a **terminal client** (`iml`) for the full feature set — plus fluent
**Subsonic / OpenSubsonic** support, so clients you already use (Supersonic,
Symfonium, DSub, and friends) work too, as a fallback.

It ships as one small **Go binary** with **SQLite** out of the box (Postgres if
you outgrow it). Drop in your music, hit play.

## What you get

- 📱 **Its own app + a terminal client** — the web app is embedded in the
  server binary, no separate install; `iml` plays music from a terminal
  without the memory footprint of a GUI app, good for gaming sessions.
- 🎧 **Also works with your Subsonic clients** — browsing, search, streaming,
  transcoding, playlists, scrobbling, now-playing.
- 🔁 **Multi-device playback** — pick up where another device left off, or
  cast to one and control it remotely, Spotify-Connect style.
- 🌍 **On-demand catalog** — pluggable providers (Jamendo, Internet Archive, and
  your own HTTP providers) stream tracks you don't own yet, progressively on
  first play.
- ✨ **Discovery & Hall of Fame** — auto-generated genre/decade/trending/chart
  playlists, personal "made for you" lists, and a hand-curated top-tracks
  ranking.
- 👯 **Social** — an activity feed with per-event privacy, and collaborative
  or public playlists.
- 🔊 **Jam sessions** — listen together, in sync, streamed live.
- 📥 **Playlist import** — bring your playlists over (Spotify ships first).
- 🔗 **Federation (opt-in)** — sync editorial & recommendation playlists via an
  `immerle-hub`.
- 🎫 **Concert discovery (opt-in)** — matches your top-listened artists
  against upcoming shows near an admin-configured country, with a Home
  banner and a notification when something new turns up.
- 🎤 **Lyrics & karaoke** — reads embedded/sidecar lyrics from your files, and
  falls back to [lrclib.net](https://lrclib.net/) for synced lyrics when a
  track has none, highlighting the current line as it plays.

## Next steps

- [Installation](./installation.md) — get a server running in a couple of
  minutes.
- [Configuration](./configuration.md) — bootstrap settings vs. runtime settings.
- [Connecting clients](./clients.md) — the app, `iml`, or any Subsonic client.
- [On-demand catalog](./on-demand-providers.md) — enable built-in providers, add your own, cleanup.
- [Discovery & Hall of Fame](./discovery.md) — auto-generated playlists and the top-tracks ranking.
- [Social features](./social.md) — activity, sharing, Jam sessions.
- [Playlist import](./playlist-import.md) — bring playlists over from Spotify or Deezer.
- [Federation](./federation.md) — sync playlists via an `immerle-hub`.
- [Developers](./developers/architecture.md) — architecture, the native &
  Subsonic APIs, and building a custom content provider.
