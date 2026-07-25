---
sidebar_position: 6
title: Concert discovery
---

# Concert discovery

An optional, runtime-configured feature (**Admin → Settings → Concert
discovery**) that matches each user's top-listened artists against upcoming
shows and surfaces the nearest one as a closable Home banner, with a toast
when a new match is found. Disabled by default: like other API-key-gated
features (Jamendo, Spotify charts), it needs configuration to be useful.

| Field | Meaning |
| ----- | ------- |
| Enable | Master switch |
| Country | The **single** country concert discovery searches near, instance-wide, not per user (a self-hosted instance is usually one household/group, not a global audience) |
| Ticketmaster API key | Optional, only shown when Ticketmaster covers the selected country |
| Skiddle API key | Optional, only shown when Skiddle covers the selected country |

How it works:

- Once a day (plus once immediately after enabling, and again whenever the
  country changes), every user's top 10 artists over the last ~180 days are
  searched against **every source that covers the selected country**, not
  an ordered fallback that stops at the first hit, so a source with thin
  coverage doesn't get starved by one that happened to find something
  unrelated.
- A match is kept per `(user, source, event)`: dismissing one is permanent,
  even after later syncs re-find it. The Home banner only ever shows the
  single nearest upcoming match.

## Sources

| Source | Needs a key | Covers |
| ------ | ----------- | ------ |
| **Ticketmaster** | Yes | US, GB, DE, ES, IT, NL, BE, IE, CA, AU, NZ, SE, NO, DK, FI, PL, AT, CH, MX, BR, TR, CZ |
| **Skiddle** | Yes | GB, IE, ES, GR, PT |
| **Eventim** | No | FR only |

Coverage is a fixed list per source, not "try everywhere": checked directly
against each API, outside these countries a source's real catalog is thin to
nonexistent (Ticketmaster's Discovery API, for instance, has essentially no
French listings, which is why Eventim exists specifically for France). A
country the admin dropdown offers is always covered by at least one source;
searching a country no source covers would silently sync nothing forever, so
that combination isn't offered. The admin UI shows which sources apply to the
currently selected country and hides API key fields for sources that don't.

:::info[Why not per-user location?]

Concert discovery originally used a per-user city field. In practice this
didn't fit a self-hosted instance (usually one household, not users spread
across cities) and free-text city names are ambiguous input for these APIs:
a single admin-configured country, matched against each provider's
structured country filter, is both simpler and more reliable.

:::
