---
sidebar_position: 6
title: Social features
---

# Social features

Beyond streaming your own library, Immerle has a small social layer: friends,
an activity feed, playlists you can share or collaborate on, and synchronized
listening sessions. All of it is opt-in per action — nothing is public by
default.

## Friends & activity

A friend relationship is a simple request/accept pair — you send a request
by username, the other person accepts it, and you're friends. Nothing else
changes about your account; it's purely a visibility gate for activity.

Plays, likes, playlist creations and similar actions feed an **activity
feed**, each event tagged with a privacy level:

| Privacy | Visible to |
| ------- | ---------- |
| Public | anyone |
| Friends | accepted friends only |
| Private | the author only |

A user's profile shows their identity, the activity they've chosen to expose
to *you* specifically, and their public playlists. Your own account page is
separate and always fully visible to you — it holds things a public profile
never exposes, like your email address.

## Collaborative & public playlists

An owner opts a playlist into two independent things:

- **Public** — visible to anyone browsing public playlists, but not
  automatically added to anyone else's library.
- **Collaborative** — specific people you choose can edit it.

Discovering a public playlist doesn't add it to your library automatically —
you **subscribe** to opt in, at which point it behaves like a normal,
read-only playlist alongside your own. A subscriber can't modify it; in a
Subsonic client, "removing" a subscribed playlist just unsubscribes you — the
owner's copy is untouched.

## Share links

A share is a **public, unauthenticated** link to a single track, album or
playlist, meant for sending to someone who doesn't have an account of their
own. It can be given an expiry date, or left open indefinitely, and the owner
can update or revoke it at any time.

## Jam sessions

A Jam is a host-controlled, synchronized listening session — everyone
listening hears the same track at the same position in real time, streamed
live. The host starts a session with a queue, anyone can join or leave
freely, but only the host can change what's playing, seek, pause, or end the
session for everyone.

---

For the exact API calls behind all of this, see the
[native API walkthrough](./developers/api-guide.md).
