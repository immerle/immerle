---
sidebar_position: 6
title: Discovery & Hall of Fame
---

# Discovery & Hall of Fame

Beyond your own library, Immerle surfaces a handful of automatically generated
and curated playlists, plus a personal top-tracks ranking. None of it needs
admin setup — it syncs itself in the background.

## Auto-generated playlists

Materialized as ordinary playlist rows (so they behave exactly like any other
playlist, in the app or over Subsonic — nothing special to configure):

- **Genre & decade playlists** ("Rock", "Rap", "1990s"…) — public, read-only,
  rebuilt daily from your local catalog.
- **"Made for you"** (Home screen, private per user) — *Top of your month*,
  *On Repeat* (last 30 days), *Forgotten favorites* (starred tracks unplayed
  for 90+ days or never played), and a random 30-track shuffle. Any list with
  zero tracks is simply skipped.
- **Weekly trending** — public, the most-scrobbled tracks across all users in
  the last 7 days.
- **Chart playlists** — a worldwide chart plus five major markets (FR, US, GB,
  DE, ES), synced weekly from Spotify's public chart data. Tracks resolve
  lazily through your on-demand providers the same way a
  [federated](./federation.md) playlist does — no provider configured just
  means those tracks aren't playable locally yet.

## Hall of Fame

A personal, hand-curated top-tracks ranking — not a playlist, its own thing.
Add any track to it from the track's context menu; the top 3 get a
gold/silver/bronze podium, the rest a ranked list below, each optionally
annotated with a short note (e.g. "listened to this in college"). Drag to
reorder.

Gated by an admin-togglable capability (`hallOfFame`, enabled by default —
see [Connecting clients](./clients.md) for how the app hides a feature
entirely the moment an admin switches it off).
