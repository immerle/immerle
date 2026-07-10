---
slug: /
sidebar_position: 1
title: Introduction
---

# Immerle

**Your music, self-hosted — and it sings.**

Immerle is a self-hosted music server that speaks fluent **Subsonic /
OpenSubsonic**, so the clients you already use (Supersonic, Symfonium, DSub, and
friends) just work. Then it goes further.

It ships as one small **Go binary** with **SQLite** out of the box (Postgres if
you outgrow it). Drop in your music, hit play.

## What you get

- 🎧 **Works with your clients** — full Subsonic / OpenSubsonic: browsing,
  search, streaming, transcoding, playlists, scrobbling, now-playing.
- 🌍 **On-demand catalog** — pluggable providers (Jamendo, Internet Archive, and
  your own HTTP providers) stream tracks you don't own yet, progressively on
  first play.
- 👯 **Social** — friends, an activity feed with per-event privacy, and
  collaborative or public playlists.
- 🔊 **Jam sessions** — listen together, in sync, streamed live.
- 📥 **Playlist import** — bring your playlists over (Spotify ships first).
- 🔗 **Federation (opt-in)** — sync editorial & recommendation playlists via an
  `immerle-hub`.

## Next steps

- [Installation](./installation.md) — get a server running in a couple of
  minutes.
- [Configuration](./configuration.md) — bootstrap settings vs. runtime settings.
- [Connecting clients](./clients.md) — point any Subsonic app at your server.
- [On-demand catalog](./on-demand-providers.md) — enable built-in providers, add your own, cleanup.
- [Social features](./social.md) — friends, activity, sharing, Jam sessions.
- [Playlist import](./playlist-import.md) — bring playlists over from Spotify or Deezer.
- [Federation](./federation.md) — sync playlists via an `immerle-hub`.
- [Developers](./developers/architecture.md) — architecture, the native &
  Subsonic APIs, and building a custom content provider.
