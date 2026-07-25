---
sidebar_position: 2
title: Bootstrap (.env)
---

# Bootstrap (`.env`)

A handful of values read from the environment (or a `.env` file) at startup.
Changing them needs a **restart**.

Copy `.env.example` to `.env`; real environment variables take precedence.

```bash
# --- HTTP server ---
PORT=4533

# --- Auth ---
# If unset, a random secret is generated at startup and persisted.
# AUTH_SECRET=
AUTH_REQUIRE_SETUP_TOKEN=false   # gate first-run admin behind a startup token (see note below)
# Optional: create the first admin from these instead of the setup UI (see note
# below). Both must be set together, or neither.
# ADMIN_USERNAME=
# ADMIN_PASSWORD=

# --- Database ---
DATABASE_DRIVER=sqlite
DATABASE_DSN=immerle.db
# For Postgres:
#   DATABASE_DRIVER=postgres
#   DATABASE_DSN=postgres://immerle:immerle@localhost:5432/immerle?sslmode=disable

# --- Logging ---
LOG_LEVEL=info     # debug | info | warn | error
# Output is always structured JSON, streamable live from the admin UI.

# --- Library ---
LIBRARY_PATHS=/music
LIBRARY_DATA_DIR=data
```

:::info[First-run admin setup]

`AUTH_REQUIRE_SETUP_TOKEN` defaults to `false` on purpose. The first time the
server starts with no users, `POST /api/v1/setup` lets you create the admin
account straight from the web UI, no token to copy out of the logs. This keeps
onboarding simple for non-technical, self-hosting users.

The setup endpoint **self-locks the moment any user exists**, so it can only be
used once. The only exposure window is between the instance first becoming
reachable on the network and you finishing setup: if someone reaches it before
you do, they could claim the admin account.

If your instance is exposed to the public internet before you've initialized it,
either set `AUTH_REQUIRE_SETUP_TOKEN=true` (the server then prints a one-time
token you must supply to create the admin) or keep the instance off the public
network until setup is complete.

For fully automated deployments (Docker, IaC) with no interactive setup step,
set `ADMIN_USERNAME`/`ADMIN_PASSWORD` instead: the server creates that admin
account at startup, before serving traffic, and skips the setup UI/token
entirely. Like the setup endpoint, this only ever applies while the server has
no users; safe to leave set permanently, it's a no-op on every later restart.

:::
