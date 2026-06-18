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
  ghcr.io/immerle/immerle:latest
```

The server comes up on **http://localhost:4533**. Create the first admin account
via the one-time setup endpoint:

```bash
curl -X POST http://localhost:4533/setup/init \
  -H 'Content-Type: application/json' \
  -d '{"username":"me","password":"a-strong-password"}'
```

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
      LOG_FORMAT: "json"
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

and create the admin with the same `/setup/init` call as above.

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
