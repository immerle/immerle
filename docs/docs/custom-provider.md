---
sidebar_position: 7
title: Custom content provider
---

# Building a custom content provider

Immerle's on-demand catalog is pluggable. A **provider** is an external source
of tracks Immerle doesn't own yet — searched, resolved and streamed
progressively on first play. Jamendo and Internet Archive ship built in; you can
add your own.

There are two kinds:

- **`builtin`** — compiled into the server (Go). Not something you add at runtime.
- **`http`** — an out-of-process service you host anywhere, in any language.
  Immerle calls fixed JSON-over-HTTP endpoints on it. **This is what you build.**

Immerle is content-neutral: it knows nothing about your catalog. It just calls a
few fixed paths under your base URL and unmarshals the JSON you return. This is a
deliberate design choice — the provider interface lets you put **any** backend
behind it, so Immerle never has to take a position on what your source is.

:::warning[Your content, your responsibility]

Because the provider system is content-neutral, it can be pointed at any source.
That neutrality is purely technical and is **not** an endorsement of any
particular use.

**You — the operator of the provider and the server — are solely responsible**
for ensuring you have the legal right to access, store and distribute whatever
content you connect, and for complying with all applicable copyright and other
laws in your jurisdiction. Immerle and its maintainers provide the mechanism
only and **disclaim all responsibility and liability** for the content you
choose to serve through it.

:::

## The contract

Immerle issues plain **`GET` requests with query params only** (no request
bodies) to fixed paths under your configured `endpoint` (any trailing slash is
stripped). Every request carries the static **headers** and **query params** you
set in the provider config (see [Config & auth](#config--auth)).

- Any response status `>= 300` is an error — **except `404`**, which on an
  optional endpoint means "not supported" and is silently treated as empty.
- Metadata responses are capped at **8 MiB**, downloads at **1 GiB**.
- Responses must be `application/json` (except `/download`, which returns raw
  audio bytes).

### The capabilities endpoint (mandatory)

Every `http` provider **must** serve `GET /capabilities`. Immerle calls it when
you add the provider (to derive its name and config form) and again to show its
live version in the admin. The response:

```json
{
  "version": 1,
  "name": "mycatalog",
  "config": {
    "apikey":        { "type": "string", "where": "params", "required": true },
    "Authorization": { "type": "string", "where": "headers", "required": false }
  }
}
```

- `version` — the protocol version you implement. Must be **`1`** (the version
  Immerle currently speaks); otherwise the provider is rejected.
- `name` — the slug Immerle stores the provider under (`^[a-z0-9][a-z0-9_-]*$`).
  **The admin doesn't type a name — this is it.**
- `config` — the config fields you accept, keyed by field name. Each declares its
  `type` (free-form, e.g. `"string"`), `where` the value travels (`"headers"` or
  `"params"`), and whether it's `required`. Immerle generates the admin's config
  form from this and, on save, rejects a config that's missing any required field.

### Other required endpoints

These three make a usable provider.

| Method & path | Query params | Response |
| ------------- | ------------ | -------- |
| `GET /search`   | `q`, `limit` | `{"results": [<Track>, …]}` |
| `GET /resolve`  | `id`         | a bare `<Track>`, or `{"result": <Track>}` |
| `GET /download` | `id`         | **raw audio bytes** (not JSON) |

- `id` is the `providerTrackId` you returned from `/search`.
- `/search` drops any track with an empty `providerTrackId`, so always set it.
- On `/download`, only the request + status is retried (3× by default); once
  bytes start flowing they are never replayed.

### The Track shape

```json
{
  "providerTrackId": "abc123",   // REQUIRED — your stable id for this track
  "title": "Song title",
  "artist": "Artist name",
  "album": "Album name",
  "albumArtist": "Album artist",
  "trackNo": 1,
  "discNo": 1,
  "year": 2024,
  "duration": 213,                // seconds
  "genre": "Electronic",
  "mbid": "",                     // MusicBrainz id, optional
  "providerArtistId": "art-42",   // optional, enables artist browsing
  "coverImageUrl": "https://cdn.example.com/cover/abc.jpg",
  "artistImageUrl": "https://cdn.example.com/artist/42.jpg",
  "suffix": "mp3"                 // file extension; defaults to "mp3"
}
```

Only `providerTrackId` is strictly required; rows without it are dropped. Image
URLs must be absolute and publicly reachable.

### Config & auth

The provider `config` is a single JSON object with this shape (every key
optional):

```json
{
  "headers": { "Authorization": "Bearer your-secret" },
  "params": { "apikey": "xyz" },
  "quality": "lossless",
  "timeoutSeconds": 60,
  "downloadRetries": 3
}
```

- **`headers`** — static HTTP headers added to every request (e.g. auth).
- **`params`** — static query params appended to every request (e.g. an API
  key as `?apikey=…`). They never override the protocol params (`q`/`limit`/`id`).
- `quality` (free-form label), `timeoutSeconds` (per-call, default 60),
  `downloadRetries` (default 3).

Put credentials in `headers` or `params` and declare them in your
[`/capabilities`](#the-capabilities-endpoint-mandatory) response so the admin
form prompts for them. The same `headers`/`params` are sent on the
`/capabilities` request too, so authenticated discovery works.

:::note[Built-in providers use the same shape]
Built-ins (Jamendo, Internet Archive…) read their tunables from `params` too —
e.g. Jamendo's config is `{"params":{"client_id":"<token>","audioformat":"mp32"}}`.
Their base URL is compiled in and is **not** configurable.
:::

### Optional endpoints (richer browsing)

Implement these to enrich artist/album pages. **Return `404` on any you don't
support** — that's how an `http` provider opts out of a capability.

| Capability   | Method & path          | Params       | Response |
| ------------ | ---------------------- | ------------ | -------- |
| Search artists | `GET /artists`       | `q`, `limit` | `{"artists": [<Artist>]}` |
| Artist albums  | `GET /artist/albums` | `id`, `limit`| `{"albums": [<Album>]}` |
| Artist tracks  | `GET /artist/tracks` | `id`, `limit`| `{"results": [<Track>]}` |
| Album tracks   | `GET /album/tracks`  | `id`, `limit`| `{"results": [<Track>]}` |
| Artist image   | `GET /artist/image`  | `name`       | `{"imageUrl": "…"}` |

```json
// <Artist>
{ "providerArtistId": "art-42", "name": "Artist", "albumCount": 3, "imageUrl": "…" }
// <Album>
{ "providerAlbumId": "alb-7", "title": "Album", "year": 2024, "coverImageUrl": "…" }
```

## A minimal provider service

Any language works; here it is in Node/Express. Implement the three required
routes, honoring your auth header.

```js
import express from 'express';
const app = express();

const AUTH = 'Bearer your-secret';

// /capabilities is public (no auth) so Immerle can discover the provider.
app.get('/capabilities', (_req, res) => {
  res.json({
    version: 1,
    name: 'mycatalog',
    config: {
      Authorization: { type: 'string', where: 'headers', required: true },
    },
  });
});

app.use((req, res, next) =>
  req.get('authorization') === AUTH ? next() : res.sendStatus(401));

app.get('/search', async (req, res) => {
  const { q, limit } = req.query;
  const hits = await myCatalog.search(q, Number(limit) || 20);
  res.json({
    results: hits.map((t) => ({
      providerTrackId: t.id,
      title: t.title,
      artist: t.artist,
      album: t.album,
      duration: t.seconds,
      coverImageUrl: t.cover,
      suffix: 'mp3',
    })),
  });
});

app.get('/resolve', async (req, res) => {
  const t = await myCatalog.get(req.query.id);
  if (!t) return res.sendStatus(404);
  res.json({ result: { providerTrackId: t.id, title: t.title, artist: t.artist, duration: t.seconds, suffix: 'mp3' } });
});

app.get('/download', async (req, res) => {
  const stream = await myCatalog.openAudio(req.query.id); // a readable stream
  res.type('audio/mpeg');
  stream.pipe(res);
});

// Opt out of browsing capabilities:
app.get(['/artists', '/artist/albums', '/artist/tracks', '/album/tracks', '/artist/image'],
  (_req, res) => res.sendStatus(404));

app.listen(8080);
```

## Registering it

Providers are admin-managed at runtime (see [Configuration](./configuration.md)).
Adding one is a three-step flow — you never type a name or config by hand.

**1. Create from the URL.** Send just the endpoint; no name, no config. The
server calls your `/capabilities`, takes the declared `name`, seeds a config
skeleton with every declared field set to `null`, and creates the provider
**disabled**:

```bash
curl -X POST http://localhost:4533/api/v1/admin/providers \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{ "endpoint": "https://my-provider.example.com" }'
```

If `/capabilities` is unreachable, advertises the wrong `version`, or returns a
non-slug `name`, the create is rejected.

**2. Fill the config.** Update the provider with the values for the declared
fields (`config` is a **JSON string**). On save, the config is validated against
`/capabilities` — a missing required field is rejected:

```bash
curl -X POST http://localhost:4533/api/v1/admin/providers \
  -H 'Authorization: Bearer <admin-token>' -H 'Content-Type: application/json' \
  -d '{
    "name": "mycatalog",
    "kind": "http",
    "endpoint": "https://my-provider.example.com",
    "config": "{\"headers\":{\"Authorization\":\"Bearer your-secret\"}}"
  }'
```

**3. Enable it.** Enabling re-runs the capability check, so a provider with an
incomplete config can't go live:

```bash
curl -X PUT http://localhost:4533/api/v1/admin/providers/mycatalog/enabled \
  -H 'Authorization: Bearer <admin-token>' -H 'Content-Type: application/json' \
  -d '{ "enabled": true }'
```

In the admin UI this is: **Add** (a dialog asking only for the URL) → the gear
(a settings panel to fill the config) → the card's switch to enable.

Other admin endpoints:

| Method & path | Purpose |
| ------------- | ------- |
| `GET /admin/providers` | List providers (with live `active`/`builtin`/`deletable`/`version`) |
| `PUT /admin/providers/order` | Reorder: `{"order": ["name1","name2",…]}` (lower = higher priority) |
| `PUT /admin/providers/{name}/enabled` | `{"enabled": <bool>}` |
| `DELETE /admin/providers/{name}` | Remove (http providers only; disable built-ins instead) |

Search and enrichment use the **first enabled provider by order**. See the full
schemas in the [API reference](pathname:///api/).
