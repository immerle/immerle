---
sidebar_position: 1
title: Quick Start
---

import QuickStartCompose from '@site/src/components/QuickStartCompose';

# Quick Start

The simplest, safest way to run Immerle: **Docker Compose**, with **Postgres**
instead of SQLite.

:::info[Not a sysadmin? Read this]

If you don't already know why "don't forward ports on your router" matters,
follow this section as-is and don't deviate: every step is chosen to be the
thing that's hardest to get wrong, not the thing with the most options. The
[Installation](../installation.md) and [Configuration](../configuration/index.md)
pages cover the other ways to run Immerle (SQLite, building from source,
running behind your own reverse proxy…) once you want more control.

:::

## Run it

Create a folder, drop your music under `music/`, and save this as
`docker-compose.yml` next to it. Music, the database and daily Postgres
backups each live in their own Docker volume rather than a host folder, more
portable if you manage this stack from Portainer, Komodo or similar. The
admin password below is generated fresh right now, reload this page for a
different one:

<QuickStartCompose />

Start it, then copy your music into the `immerle-music` volume:

```bash
docker compose up -d
docker run --rm -v immerle-music:/dest -v "$PWD/music":/src:ro alpine cp -a /src/. /dest/
```

Open `http://localhost:4533` on the same machine and sign in with `admin` /
the password from the `ADMIN_PASSWORD` line above. That's it: the server,
database and your library are all up, with the last 7 daily database backups
always kept in the `immerle_backup` volume.

## Next

- [Choosing a client](./choosing-a-client.md): connect and pick between the
  Immerle app, `iml`, or a Subsonic client.
