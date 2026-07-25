---
sidebar_position: 7
title: Scrobbling
---

# Scrobbling

Every submitted play can be pushed to **ListenBrainz** and/or **Last.fm**,
in addition to Immerle's own local play history. Both are per-user and opt-in
(**Settings → Scrobble & suggestions**); connecting one doesn't require or
affect the other, and a user can connect both at once — every play then fans
out to whichever services they've set up.

## ListenBrainz

No admin setup needed. A user pastes their own personal token (found on
[listenbrainz.org/settings](https://listenbrainz.org/settings)) into
**Settings → Scrobble & suggestions**; it's validated live against
ListenBrainz before being saved. The same token also powers the
ListenBrainz-generated *Daily Jams*/*Weekly Jams*/*Weekly Exploration*
playlists described in [Discovery](./discovery.md), when ListenBrainz has
generated them for that user.

## Last.fm

Last.fm's API requires an app-level key + shared secret (there's no
per-user pasted-token equivalent), so this one needs a short admin setup
first:

1. Register an application at
   [last.fm/api/account/create](https://www.last.fm/api/account/create) —
   any values work for the name/description/homepage, and the **callback
   URL** field specifically is not used by Immerle's flow (see below), so
   any valid URL satisfies the form.
2. **Admin → Settings → Last.fm**: enable it and paste the API key and
   shared secret Last.fm issued. Like the other API-key-gated features
   (Concert discovery, Jamendo), it's disabled by default and hidden from
   users until configured.

Once enabled, each user connects their own account from **Settings →
Scrobble & suggestions**:

1. **Connect Last.fm** opens Last.fm's authorization page in the browser.
2. After approving access there, the user comes back to Immerle and taps
   **I've approved it** to complete the connection.

This is Last.fm's "desktop" auth flow (a token is approved manually, then
exchanged for a permanent session key) rather than the "web application"
flow (an automatic redirect back via a callback URL). It was chosen
deliberately: it works identically on web and native, and doesn't depend on
a self-hosted instance having a fixed, publicly reachable URL to redirect
back to.

## Disconnecting

Either service can be disconnected independently from **Settings → Scrobble
& suggestions**; this only stops future scrobbles; it doesn't retract
anything already submitted (neither ListenBrainz nor Last.fm support that).
