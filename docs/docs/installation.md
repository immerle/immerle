---
sidebar_position: 2
title: Installation
---

# Installation

Immerle is a single Go binary. Run it with Docker, or build it from source.

## Docker (recommended)

Run the published image, pointing it at your music folder:

```bash
docker run -d --name immerle \
  -p 4533:4533 \
  -v ./music:/music:ro \
  -v immerle-data:/data \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=change-me \
  ghcr.io/immerle/immerle:latest
```

The server comes up on **http://localhost:4533**, with that admin account
already created — sign in with it right away. Omit the two `ADMIN_*`
variables to get an interactive setup screen in the web UI instead. See
[Configuration](./configuration.md) for the full list of bootstrap variables.

### Docker Compose

For a persistent setup, a Compose file is more convenient:

```yaml title="docker-compose.yml"
services:
  immerle:
    image: ghcr.io/immerle/immerle:latest
    ports:
      - "4533:4533"
    environment:
      PORT: "4533"
      DATABASE_DRIVER: "sqlite"
      DATABASE_DSN: "/data/immerle.db"
      LIBRARY_DATA_DIR: "/data"
      LIBRARY_PATHS: "/music"
      AUTH_REQUIRE_SETUP_TOKEN: "false"
      # ADMIN_USERNAME: "admin"
      # ADMIN_PASSWORD: "change-me"
    volumes:
      - ./music:/music:ro
      - immerle-data:/data
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:4533/ping"]
      interval: 30s
      timeout: 3s
      retries: 3
    restart: unless-stopped

volumes:
  immerle-data:
```

Then:

```bash
docker compose up -d
```

Uncomment `ADMIN_USERNAME`/`ADMIN_PASSWORD` above to have the admin account
created automatically, same as the plain `docker run` example.

## From source

You'll need **Go 1.25+** and `ffmpeg` / `ffprobe` on your `PATH` (for
transcoding, duration probing and on-demand tag embedding).

```bash
make build
cp .env.example .env   # edit as needed
./bin/immerle          # auto-loads .env (or pass -env path/to/.env)
```

## Verify

Once it's running, point any Subsonic client at `http://<host>:4533` with the
credentials you just created — see [Connecting clients](./clients.md).

:::tip[Deploying beyond localhost]

For anything past local testing, put a TLS-terminating reverse proxy (Traefik,
Caddy, nginx…) in front of Immerle rather than exposing port 4533 directly —
it also gets you HTTP/2 for free, which matters once you use live features
like Jam. See [Troubleshooting](./troubleshooting.md#pages-feel-slow--stall-while-a-jam-is-running)
for why that's worth doing and how it works.

:::
