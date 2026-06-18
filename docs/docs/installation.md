---
sidebar_position: 2
title: Installation
---

# Installation

Immerle is a single Go binary. Run it with Docker, or build it from source.

## Docker (recommended)

Put your music under `./music`, then:

```bash
docker compose up --build
```

The server comes up on **http://localhost:4533**. Create the first admin account
via the one-time setup endpoint:

```bash
curl -X POST http://localhost:4533/setup/init \
  -H 'Content-Type: application/json' \
  -d '{"username":"me","password":"a-strong-password"}'
```

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
