---
sidebar_position: 3
title: Configuration
---

# Configuration

Immerle splits configuration in two:

- **Bootstrap settings** — a handful of values read from the environment (or a
  `.env` file) at startup. Changing them needs a **restart**.
- **Runtime settings** — everything else (providers, scan cadence, transcoding,
  CORS, device-token TTL, federation…), managed by an admin via the API and
  stored in `data/configuration.yaml`. No restart needed.

## Bootstrap (`.env`)

Copy `.env.example` to `.env`; real environment variables take precedence.

```bash
# --- HTTP server ---
PORT=4533

# --- Auth ---
# If unset, a random secret is generated at startup and persisted.
# AUTH_SECRET=
AUTH_REQUIRE_SETUP_TOKEN=false   # gate first-run admin behind a startup token

# --- Database ---
DATABASE_DRIVER=sqlite
DATABASE_DSN=immerle.db
# For Postgres:
#   DATABASE_DRIVER=postgres
#   DATABASE_DSN=postgres://immerle:immerle@localhost:5432/immerle?sslmode=disable

# --- Logging ---
LOG_LEVEL=info     # debug | info | warn | error
LOG_FORMAT=text    # text | json

# --- Library ---
LIBRARY_PATHS=/music
LIBRARY_DATA_DIR=data
```

## Runtime (admin API)

Runtime settings are managed via the admin API and persisted in
`data/configuration.yaml`:

| Area      | Endpoint                |
| --------- | ----------------------- |
| Settings  | `GET/POST /admin/settings`  |
| Providers | `GET/POST /admin/providers` |
| Cleanup   | `GET/POST /admin/cleanup`   |

Providers (including built-ins like Jamendo and their credentials) are **not**
set in `.env`. Jamendo, for instance, is seeded disabled with a
`{"params":{"client_id":"<token>"}}` config to fill in and enable from the admin
UI. See [Custom content provider](./custom-provider.md) for the config schema and
the `/capabilities` contract used to add an HTTP provider.
