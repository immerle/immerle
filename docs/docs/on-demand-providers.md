---
sidebar_position: 5
title: On-demand catalog
---

# On-demand catalog

Beyond your own library, Immerle can search and stream tracks you don't own
yet from pluggable **providers** — legal, no-DRM sources. The first listen
streams progressively while the track is saved in the background; every later
play is local, transcoded and seekable like anything else in your library.

The on-demand catalog is always running: with no provider enabled it simply
has nothing to search, equivalent to being off. There's no separate on/off
switch for the feature itself — you manage it by enabling and disabling
individual providers from the admin settings.

## Built-in providers

Shipped compiled into the binary, configured entirely from the admin
settings — credentials for a built-in live in its config there, never in
`.env`:

| Provider | Catalog | Credentials | Default |
| -------- | ------- | ------------ | ------- |
| Jamendo | Creative Commons, free authorized downloads | Client ID required | seeded, disabled until configured |
| Internet Archive | Public domain, CC and artist-sanctioned live recordings | none | enabled |
| Free Music Archive | freemusicarchive.org CC catalog | none | enabled, first in priority order |

Providers have a **priority order**, editable from the admin settings — the
first *enabled* one is the one search and enrichment use. There's no separate
"default provider" setting; reordering the list is how you choose.

## Dynamic (HTTP) providers

Beyond the built-in ones, an admin can register **any HTTP service** as a
provider — no rebuild, no restart. Immerle calls a small fixed set of
endpoints on it and is otherwise content-neutral: it doesn't know or care
what's behind the URL you point it at.

This is the seam for plugging in a catalog you operate yourself — see
[Building a custom content provider](./developers/custom-provider.md) if
you're writing one. A provider's name comes from its own capabilities
response, not from what you type when adding it. Built-ins can be
reconfigured, disabled and reordered the same way as a dynamic provider, but
never deleted.

## Progressive first play

Streaming a remote track for the first time doesn't wait for a full download:
the provider's bytes are teed to the client and to disk simultaneously, so
playback starts immediately. That first stream is the provider's original
audio, untranscoded — transcoding needs the whole file buffered first. The
saved copy is tagged and scanned into your library in the background, so the
next play resolves locally through the normal transcoding/seekable path.

Search results merge your local library with the top-priority enabled
provider's results, deduplicated. A local artist's page is also enriched with
the rest of their discography from that provider — browsable and streamable
on demand, even for albums you don't own.

Seeking is disabled (with an explanatory hint in the app) for as long as a
track is still streaming progressively — every play of an undownloaded track
starts a fresh download from byte 0, so jumping ahead would just restart it.
Once the background download finishes, replaying the track serves it locally
and seeking works normally.

## Artist avatars

Artist pictures come from the same providers as everything else on demand —
there's no separate avatar feature to enable. If an enabled provider can
supply artist images, one is resolved, validated, cached locally and shown
wherever an artist's picture appears. If none can, artists simply have no
avatar; there's no toggle for this, it just works whenever a provider
supports it.

## Cleanup of unused downloads

Provider-downloaded tracks don't accumulate forever. A background sweep
(on by default: 30-day window, every 6 hours) deletes a downloaded track's
file and database rows only when there's **no reason to keep it** — unplayed
within the window, in no playlist, starred by nobody. Anything you added
manually is never touched, since there's no download job behind it to key
the sweep on. The sweep can be toggled or run on demand from the admin
settings.

---

For the exact API calls behind all of this, see the
[native API walkthrough](./developers/api-guide.md).
