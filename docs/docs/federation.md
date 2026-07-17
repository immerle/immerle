---
sidebar_position: 9
title: Federation
---

# Federation

Federation links your instance to an `immerle-hub` — an optional, opt-in
connection that syncs playlists between instances, resolves tracks by a
portable id even when you don't have the source instance's exact file, and
lets you export anonymized listening stats. Nothing is federated until you
link.

There's no on/off toggle to look for: federation is active whenever your
instance is linked, and idle otherwise.

## Linking

Linking is a two-step, admin-only process: you register your hub account
with your instance, then the server does a one-time handshake with the hub
that issues your instance a fixed identity and a unique, editable public
handle. That identity is what lets other instances find and subscribe to
you.

The hub's own address is fixed by Immerle and isn't something you configure
— only the parts specific to *your* instance are.

## What syncs

Two independent things, each with its own switch in the admin settings:

- **Playlist sync** — your public, non-federated playlists are pushed to the
  hub, so they can appear on other linked instances. Identical cover art
  across instances isn't re-uploaded.
- **Scrobble export** — your listening counts are aggregated per track, with
  identity and timestamps dropped, so the hub can build aggregate stats
  without ever seeing which instance or user played what.

A federated playlist can reference a track by a portable identifier your
instance doesn't have a local file for. Tapping it checks your library for
that id and, if it's missing, searches your configured on-demand providers —
no separate toggle. With no provider configured, such tracks just aren't
playable locally.

## Discovering other instances

Once linked, you can search for other instances on the hub and subscribe to
them. Subscribing surfaces their federated (public) playlists in your own
library, read-only — the same way subscribing to a public playlist on your
own server works.

---

For the exact API calls behind all of this, see the
[native API walkthrough](./developers/api-guide.md).
