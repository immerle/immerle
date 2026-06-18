---
sidebar_position: 6
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

:::warning Your content, your responsibility

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
stripped). Every request carries the headers you set in the provider config (see
[Auth](#auth)).

- Any response status `>= 300` is an error — **except `404`**, which on an
  optional endpoint means "not supported" and is silently treated as empty.
- Metadata responses are capped at **8 MiB**, downloads at **1 GiB**.
- Responses must be `application/json` (except `/download`, which returns raw
  audio bytes).

### Required endpoints

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

### Auth

There is no separate API-key mechanism for `http` providers — put credentials in
**headers**, set in the provider's `config` blob. They are sent verbatim on
every request:

```json
{ "headers": { "Authorization": "Bearer your-secret" } }
```

Other config keys (all optional): `quality` (free-form label), `timeoutSeconds`
(per-call, default 60), `downloadRetries` (default 3).

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
Register yours via the admin API:

```bash
curl -X POST http://localhost:4533/api/v1/admin/providers \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "mycatalog",
    "kind": "http",
    "endpoint": "https://my-provider.example.com",
    "config": "{\"headers\":{\"Authorization\":\"Bearer your-secret\"}}",
    "enabled": true
  }'
```

- `name` must match `^[a-z0-9][a-z0-9_-]{0,62}$` and be unique.
- `config` is a **JSON string** (note the escaped quotes).
- The provider is built once at registration to reject a bad endpoint/config
  before it's saved, then placed at the front of the priority order.

Other admin endpoints:

| Method & path | Purpose |
| ------------- | ------- |
| `GET /admin/providers` | List providers (with live `active`/`builtin`/`deletable`) |
| `PUT /admin/providers/order` | Reorder: `{"order": ["name1","name2",…]}` (lower = higher priority) |
| `PUT /admin/providers/{name}/enabled` | `{"enabled": <bool>}` |
| `DELETE /admin/providers/{name}` | Remove (http providers only; disable built-ins instead) |

Search and enrichment use the **first enabled provider by order**. See the full
schemas in the [API reference](pathname:///api/).
