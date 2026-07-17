---
sidebar_position: 4
title: Connecting clients
---

# Connecting clients

Immerle ships two clients of its own — the **app** (web, iOS, Android) and
**`iml`** (terminal) — both built against Immerle's native API. That's where
the full feature set lives: on-demand catalog & providers, Jam sessions,
federation, Hall of Fame, discovery playlists, playlist import, offline
downloads, live admin tools, and everything else this site documents.

Subsonic/OpenSubsonic is also fully supported, so any client for that
ecosystem works too — but the Subsonic API is an older, fixed protocol that
was never designed for most of the above, so a Subsonic client only ever sees
the "plain music server" subset. Treat it as a fallback for when you'd rather
keep using an app you already have.

## The Immerle app (recommended)

One codebase, three targets: **web, iOS, Android**.

- **Web** needs nothing extra — the server embeds and serves it directly at
  `http://<host>:4533`. Sign in with the account you created during
  [installation](./installation.md) and you're done.
- **iOS / Android** aren't published to an app store yet; build them yourself
  with EAS (`npx eas build --profile production --platform ios|android`) or
  run a dev client locally. See `ui/README.md` in the repo for the exact
  commands.

It's *capability-aware*: it probes the server on connect and only shows the
features that instance actually has enabled, so it degrades gracefully
against an older server or one with things turned off — no broken buttons.

### Multi-device playback

Sign in on more than one device and the app keeps them in sync: the
play queue, current track and position are shared, so picking up your phone
resumes wherever your laptop left off. From the player, "cast to device"
picks one device as the sole active player (the others pause instead of
doubling audio), and the picking device keeps play/pause/skip/seek control
over it remotely — same idea as Spotify Connect. Pick "Everywhere" to go back
to every device playing independently.

## `iml` — terminal client

A UI-less TUI: search songs/albums/playlists and play them without ever
leaving the terminal. It renders text, not a GUI, so it barely touches memory
or CPU — handy for just having music running in the background without
competing with a game or anything else demanding for resources.

Install it (Go 1.25+ needed to build):

```bash
make install-cli   # go install ./cmd/iml — lands `iml` on your $GOBIN/$PATH
```

Run `iml`, enter your server URL and credentials once — the session is saved
to `~/.immerle/config.json` and reused after that (`iml logout` clears it).

| Key | Does |
| --- | --- |
| type | search (matches songs, albums *and* playlists) |
| `/song`, `/album`, `/playlist` | scope the search to one type |
| `↑` / `↓`, `Enter` | move the selection, play it |
| `Space` | play / pause |
| `n` | next track |
| `+` / `-` | volume up / down |
| `r` | cycle repeat: off → all → one |
| `s` | toggle shuffle |
| `q` | quit |

## Subsonic / OpenSubsonic clients

Point any compatible client at:

- **Server / URL:** `http://<host>:4533`
- **Username / Password:** your Immerle credentials

Tested:

- [Supersonic](https://github.com/dweymouth/supersonic) (desktop)
- [Symfonium](https://symfonium.app/) (Android)
- [DSub](https://github.com/daneren2005/Subsonic) (Android)

Any other Subsonic/OpenSubsonic client should work for browsing, search,
streaming, transcoding, playlists and scrobbling — just without the
Immerle-only features listed above. If something doesn't work,
[open an issue](https://github.com/immerle/immerle/issues).
