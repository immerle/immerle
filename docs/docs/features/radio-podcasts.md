---
sidebar_position: 9
title: Radio & podcasts
---

# Radio & podcasts

Beyond music files, Immerle plays live internet radio and podcasts, both from
the app.

![The Radio screen, stations grouped by country](/img/screenshots/radio.webp)

## Internet radio

A curated set of stations ships **embedded in the binary**, grouped by country
(France, Spain, UK, US, Switzerland, plus an international group), each with
its logo bundled so nothing is hotlinked at runtime. Stations show up on their
own Radio screen and in the global search; each user can like stations to
keep them in a favorites list.

An admin can add, edit or delete stations from the admin settings, and switch
the whole feature off (it's on by default; the app hides Radio entirely when
it's off).

Adding a built-in station or country to Immerle itself is a one-folder change
in the repo: a `stations.json` plus a `covers/` folder under `radio/<country>/`.

## Podcasts

Podcasts are subscribed **instance-wide by an admin**, then listened to by
everyone: the server fetches each RSS feed, keeps the episode list refreshed,
and streams episodes straight from the publisher. An episode can also be
downloaded to the server so it plays locally from then on.

To find a feed, the admin searches one of the built-in podcast directories
(discovery only, the audio always comes from the publisher's own feed), or
pastes a feed URL directly:

| Directory | Credentials |
| --------- | ----------- |
| Apple Podcasts | none |
| fyyd | none |
| gpodder.net | none |
| Podcast Index | API key + secret |
| Listen Notes | API key |

Directories are enabled and configured from the admin settings, like on-demand
providers. For the exact endpoints, see the
[OpenAPI reference](pathname:///api/) (`/podcasts*`, `/admin/podcasts*`).
